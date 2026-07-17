package engine_test

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"composable-operations/internal/core"
	"composable-operations/internal/engine"
	"composable-operations/internal/llm"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
)

// WorkflowSuite covers the key behaviors of RunFlow as specified in the design.
type WorkflowSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env       *testsuite.TestWorkflowEnvironment
	reg       *registry.Registry
	workflows *engine.Workflows
}

func (s *WorkflowSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.reg = registry.New()
	s.Require().NoError(ops.RegisterBuiltins(s.reg, &llm.StubClient{}))
	s.workflows = &engine.Workflows{Registry: s.reg}
	s.env.RegisterWorkflowWithOptions(s.workflows.RunFlow, workflow.RegisterOptions{Name: "RunFlow"})
	acts := &engine.Activities{Registry: s.reg}
	s.env.RegisterActivity(acts)
}

func (s *WorkflowSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

func TestWorkflowSuite(t *testing.T) {
	suite.Run(t, new(WorkflowSuite))
}

// kb1: steps execute in order; kb2: each step receives previous output.
func (s *WorkflowSuite) TestRunFlow_ExecutesStepsInOrderAndThreadsEnvelope() {
	order := make([]string, 0)

	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.MatchedBy(func(in engine.ActivityInput) bool {
		return in.Step.ID == "step-a"
	})).Return(func(_ context.Context, in engine.ActivityInput) (engine.ActivityOutput, error) {
		order = append(order, "step-a")
		out := copyEnv(in.Envelope)
		out["a_done"] = true
		return engine.ActivityOutput{Envelope: out}, nil
	})

	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.MatchedBy(func(in engine.ActivityInput) bool {
		return in.Step.ID == "step-b"
	})).Return(func(_ context.Context, in engine.ActivityInput) (engine.ActivityOutput, error) {
		order = append(order, "step-b")
		// Verify the envelope contains what step-a wrote (kb2: threading)
		s.Require().Equal(true, in.Envelope["a_done"], "step-b must receive step-a output")
		out := copyEnv(in.Envelope)
		out["b_done"] = true
		return engine.ActivityOutput{Envelope: out}, nil
	})

	def := twoStepActivityDef()
	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: def,
		Input:      core.Envelope{"trigger": true},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
	s.Equal([]string{"step-a", "step-b"}, order)

	var result core.Envelope
	s.Require().NoError(s.env.GetWorkflowResult(&result))
	s.Equal(true, result["b_done"])
}

// kb3: human.approval suspends the run.
// kb4: on approve, attaches decision and continues.
func (s *WorkflowSuite) TestRunFlow_HumanApproval_ApprovesContinues() {
	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.Anything).
		Return(func(_ context.Context, in engine.ActivityInput) (engine.ActivityOutput, error) {
			out := copyEnv(in.Envelope)
			out["published"] = true
			return engine.ActivityOutput{Envelope: out}, nil
		})

	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
			StepID:   "human-review",
			Approved: true,
			Comment:  "Looks good",
			By:       "human",
		})
	}, 0)

	def := humanGateDef()
	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: def,
		Input:      core.Envelope{"content": "test"},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result core.Envelope
	s.Require().NoError(s.env.GetWorkflowResult(&result))
	decision := result["decision"].(map[string]any)
	s.Equal(true, decision["approved"])
	s.Equal("human", decision["by"])
}

// kb5: on reject, run fails and does not execute downstream steps.
func (s *WorkflowSuite) TestRunFlow_HumanApproval_RejectStopsRun() {
	publishCalled := false
	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.MatchedBy(func(in engine.ActivityInput) bool {
		if in.Step.ID == "publish-content" {
			publishCalled = true
		}
		return true
	})).Return(engine.ActivityOutput{Envelope: core.Envelope{}}, nil).Maybe()

	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
			StepID:   "human-review",
			Approved: false,
			Comment:  "Not suitable",
			By:       "human",
		})
	}, 0)

	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: humanGateDef(),
		Input:      core.Envelope{"content": "bad content"},
	})

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
	s.False(publishCalled, "publish must not be called when rejected")
}

// kb14: signal with wrong step_id is ignored.
func (s *WorkflowSuite) TestRunFlow_HumanApproval_IgnoresWrongStepID() {
	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.Anything).
		Return(engine.ActivityOutput{Envelope: core.Envelope{"published": true}}, nil)

	// First signal has wrong step_id and should be ignored; second is correct.
	callCount := 0
	s.env.RegisterDelayedCallback(func() {
		callCount++
		if callCount == 1 {
			s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
				StepID:   "wrong-step",
				Approved: false,
				By:       "human",
			})
			s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
				StepID:   "human-review",
				Approved: true,
				By:       "human",
			})
		}
	}, 0)

	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: humanGateDef(),
		Input:      core.Envelope{"content": "test"},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
}

// kb9: run uses definition from its input, not from disk (immutable mid-run).
func (s *WorkflowSuite) TestRunFlow_UsesDefinitionFromInput() {
	s.env.OnActivity(engine.ExecuteOperationName, mock.Anything, mock.MatchedBy(func(in engine.ActivityInput) bool {
		return in.Step.Type == "pii.scan"
	})).Return(engine.ActivityOutput{Envelope: core.Envelope{"pii_found": false}}, nil)

	frozenDef := core.FlowDefinition{
		Name: "frozen",
		Steps: []core.StepConfig{
			{ID: "scan", Type: "pii.scan", Params: map[string]any{"patterns": []any{`\d+`}}},
		},
	}

	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: frozenDef,
		Input:      core.Envelope{},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
}

// --- helpers ---

func twoStepActivityDef() core.FlowDefinition {
	return core.FlowDefinition{
		Name: "two-step",
		Steps: []core.StepConfig{
			{ID: "step-a", Type: "pii.scan", Params: map[string]any{"patterns": []any{`\d+`}}},
			{ID: "step-b", Type: "pii.scan", Params: map[string]any{"patterns": []any{`\d+`}}},
		},
	}
}

func humanGateDef() core.FlowDefinition {
	return core.FlowDefinition{
		Name: "moderation",
		Steps: []core.StepConfig{
			{
				ID:   "human-review",
				Type: "human.approval",
				Params: map[string]any{
					"prompt": "Review this content",
				},
			},
			{
				ID:     "publish-content",
				Type:   "publish",
				Params: map[string]any{},
			},
		},
	}
}

func copyEnv(e core.Envelope) core.Envelope {
	out := make(core.Envelope, len(e))
	maps.Copy(out, e)
	return out
}
