package engine_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"composable-operations/internal/core"
	"composable-operations/internal/engine"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
	"composable-operations/internal/testutil"
)

// IntegrationSuite runs full end-to-end flow chains through the real registry
// and real ops (no activity mocks), using StubChatModel as the LLM. This is
// the safety net that catches op-level bugs that unit tests miss because they
// control LLM responses directly.
type IntegrationSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env       *testsuite.TestWorkflowEnvironment
	workflows *engine.Workflows
}

func (s *IntegrationSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	reg := registry.New()
	s.Require().NoError(ops.RegisterBuiltins(reg, &testutil.StubChatModel{}))
	s.workflows = &engine.Workflows{Registry: reg}
	acts := &engine.Activities{Registry: reg}
	s.env.RegisterWorkflowWithOptions(s.workflows.RunFlow, workflow.RegisterOptions{Name: "RunFlow"})
	s.env.RegisterActivity(acts)
}

func (s *IntegrationSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationSuite))
}

// v1: human approves → full chain completes, content remediated.
func (s *IntegrationSuite) TestIncidentResponseV1_HumanApproves_Remediates() {
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
			StepID:   "human-review",
			Approved: true,
			Comment:  "Approved by on-call",
			By:       "human",
		})
	}, 0)

	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: incidentV1Def(),
		Input:      core.Envelope{"trigger": "alert", "alert_id": "ALT-001"},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	s.Require().NoError(s.env.GetWorkflowError())

	var result core.Envelope
	s.Require().NoError(s.env.GetWorkflowResult(&result))

	s.Equal(true, result["remediated"], "remediation must complete on approval")
	decision, ok := result["decision"].(map[string]any)
	require.True(s.T(), ok)
	s.Equal(true, decision["approved"])
	s.Equal("human", decision["by"])
	s.NotNil(result["analysis"], "analysis must be present in final envelope")
}

// v1: human rejects → run fails without remediating.
func (s *IntegrationSuite) TestIncidentResponseV1_HumanRejects_RunFails() {
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(engine.SignalApproval, core.ApprovalDecision{
			StepID:   "human-review",
			Approved: false,
			Comment:  "False alarm",
			By:       "human",
		})
	}, 0)

	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: incidentV1Def(),
		Input:      core.Envelope{"trigger": "alert"},
	})

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError(), "run must fail when human rejects")
}

// v2: LLM decides autonomously → same final shape, by="llm".
func (s *IntegrationSuite) TestIncidentResponseV2_LLMDecides_Remediates() {
	s.env.ExecuteWorkflow(s.workflows.RunFlow, engine.FlowInput{
		Definition: incidentV2Def(),
		Input:      core.Envelope{"trigger": "alert", "alert_id": "ALT-002"},
	})

	s.Require().True(s.env.IsWorkflowCompleted())
	s.Require().NoError(s.env.GetWorkflowError())

	var result core.Envelope
	s.Require().NoError(s.env.GetWorkflowResult(&result))

	s.Equal(true, result["remediated"])
	decision, ok := result["decision"].(map[string]any)
	require.True(s.T(), ok)
	s.Equal(true, decision["approved"])
	s.Equal("llm", decision["by"], "v2 decision must come from LLM, not human")
}

// --- flow definitions ---

func incidentV1Def() core.FlowDefinition {
	return core.FlowDefinition{
		Name: "incident-response",
		Steps: []core.StepConfig{
			{
				ID:   "check-metrics",
				Type: "metrics.check",
				Params: map[string]any{
					"fixture": map[string]any{
						"service":        "payment-api",
						"cpu_usage":      0.92,
						"error_rate":     0.15,
						"p99_latency_ms": float64(2400),
					},
				},
			},
			{
				ID:   "check-logs",
				Type: "logs.check",
				Params: map[string]any{
					"fixture": []any{
						"ERROR payment-api: timeout connecting to postgres after 30s",
						"WARN  payment-api: circuit breaker open for downstream db",
					},
				},
			},
			{
				ID:   "analyze-incident",
				Type: "llm.analyze",
				Params: map[string]any{
					"prompt_template": `Analyze service {{.service}} with CPU {{.cpu_usage}} and error rate {{.error_rate}}. Respond with JSON only: {"root_cause": string, "severity": string, "recommended_action": string}`,
				},
			},
			{
				ID:   "human-review",
				Type: "human.approval",
				Params: map[string]any{
					"prompt":         "Review incident and approve remediation.",
					"display_fields": []any{"service", "analysis"},
				},
			},
			{
				ID:     "remediate",
				Type:   "remediate",
				Params: map[string]any{"endpoint": "https://ops.internal/remediate"},
			},
		},
	}
}

func incidentV2Def() core.FlowDefinition {
	def := incidentV1Def()
	// Replace human-review with llm-review.
	def.Steps[3] = core.StepConfig{
		ID:   "llm-review",
		Type: "llm.decision",
		Params: map[string]any{
			"prompt_template": `Incident on {{.service}}: {{index .analysis "root_cause"}}. Should we auto-remediate? Respond with JSON only: {"approved": bool, "comment": string}`,
		},
	}
	return def
}
