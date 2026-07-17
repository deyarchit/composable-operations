package engine

import (
	"fmt"
	"maps"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"composable-operations/internal/core"
	"composable-operations/internal/registry"
)

const (
	// QueryStatus is the Temporal query name used by GET /runs/{id}.
	QueryStatus = "status"
	// SignalApproval is the single well-known signal name for human gate decisions.
	SignalApproval = "approval"
	// TaskQueue is the Temporal task queue name.
	TaskQueue = "composable-operations"
)

// FlowInput is the workflow input: the full definition plus the trigger envelope.
// Passing the definition as workflow input (not looked up at runtime) ensures a
// run always executes the version it started with even if the YAML changes.
type FlowInput struct {
	Definition core.FlowDefinition
	Input      core.Envelope
}

// Workflows holds the registry needed by RunFlow to dispatch human gates.
// It is injected at worker startup so workflows avoid global state.
type Workflows struct {
	Registry *registry.Registry
}

// RunFlow is the generic chain interpreter. It executes steps in declared order,
// threading the payload envelope. Activity steps go through ExecuteOperation;
// human gate steps block on the approval signal.
func (w *Workflows) RunFlow(ctx workflow.Context, input FlowInput) (core.Envelope, error) {
	envelope := input.Input
	steps := initSteps(input.Definition.Steps)
	var pendingApproval *core.ApprovalRequest

	if err := workflow.SetQueryHandler(ctx, QueryStatus, func() (*core.RunStatus, error) {
		return buildRunStatus(workflow.GetInfo(ctx).WorkflowExecution.ID, input.Definition.Name, steps, envelope, pendingApproval), nil
	}); err != nil {
		return nil, fmt.Errorf("register query handler: %w", err)
	}

	for i, step := range input.Definition.Steps {
		steps[i].Status = core.StepRunning

		var err error
		envelope, err = w.executeStep(ctx, step, envelope, input.Definition.Name, &pendingApproval)
		if err != nil {
			steps[i].Status = core.StepFailed
			steps[i].Error = err.Error()
			return envelope, err
		}

		steps[i].Status = core.StepCompleted
		steps[i].Output = copyMap(envelope)
	}

	return envelope, nil
}

func (w *Workflows) executeStep(ctx workflow.Context, step core.StepConfig, envelope core.Envelope, flowName string, pendingApproval **core.ApprovalRequest) (core.Envelope, error) {
	op, ok := w.Registry.Get(step.Type)
	if !ok {
		return envelope, fmt.Errorf("unknown op type %q", step.Type)
	}

	switch op.Kind() {
	case core.KindActivity:
		return w.executeActivityStep(ctx, step, envelope)
	case core.KindHumanGate:
		gate, ok := op.(core.HumanGate)
		if !ok {
			return envelope, fmt.Errorf("op %q claims HumanGate kind but does not implement HumanGate interface", step.Type)
		}
		return w.executeHumanGateStep(ctx, step, envelope, flowName, gate, pendingApproval)
	default:
		return envelope, fmt.Errorf("op %q has unsupported kind %q", step.Type, op.Kind())
	}
}

func (w *Workflows) executeActivityStep(ctx workflow.Context, step core.StepConfig, envelope core.Envelope) (core.Envelope, error) {
	var out ActivityOutput
	ao := activityOptions(step)
	if err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, ao),
		ExecuteOperationName,
		ActivityInput{Step: step, Envelope: envelope},
	).Get(ctx, &out); err != nil {
		return envelope, fmt.Errorf("step %q failed: %w", step.ID, err)
	}
	return out.Envelope, nil
}

func (w *Workflows) executeHumanGateStep(ctx workflow.Context, step core.StepConfig, envelope core.Envelope, flowName string, gate core.HumanGate, pendingApproval **core.ApprovalRequest) (core.Envelope, error) {
	req := gate.BuildRequest(envelope, step.Params)
	info := workflow.GetInfo(ctx)
	req.RunID = info.WorkflowExecution.ID
	req.StepID = step.ID
	req.Flow = flowName
	*pendingApproval = &req

	decision := awaitMatchingApproval(ctx, step.ID)

	*pendingApproval = nil

	if !decision.Approved {
		return envelope, fmt.Errorf("step %q rejected: %s", step.ID, decision.Comment)
	}
	return cloneWithDecision(envelope, decision), nil
}

// awaitMatchingApproval blocks on the approval signal channel until a decision
// whose StepID matches stepID arrives. Mismatched signals (stale or wrong step)
// are silently discarded.
func awaitMatchingApproval(ctx workflow.Context, stepID string) core.ApprovalDecision {
	ch := workflow.GetSignalChannel(ctx, SignalApproval)
	var decision core.ApprovalDecision
	for {
		ch.Receive(ctx, &decision)
		if decision.StepID == stepID {
			return decision
		}
	}
}

func activityOptions(step core.StepConfig) workflow.ActivityOptions {
	// Default to 3 attempts so non-transient errors (parse failures, bad config)
	// eventually fail the run rather than retrying forever.
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 60 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	}
	if rp := step.RetryPolicy; rp != nil {
		ao.RetryPolicy.MaximumAttempts = int32(rp.MaxAttempts) //nolint:gosec // MaxAttempts is a small positive int from config
		if rp.InitialIntervalSeconds > 0 {
			ao.RetryPolicy.InitialInterval = time.Duration(rp.InitialIntervalSeconds * float64(time.Second))
		}
	}
	return ao
}

func initSteps(steps []core.StepConfig) []core.StepResult {
	results := make([]core.StepResult, len(steps))
	for i, s := range steps {
		results[i] = core.StepResult{StepID: s.ID, Status: core.StepPending}
	}
	return results
}

func buildRunStatus(runID, flow string, steps []core.StepResult, envelope core.Envelope, pending *core.ApprovalRequest) *core.RunStatus {
	state := deriveRunState(steps, pending)
	status := &core.RunStatus{
		RunID: runID,
		Flow:  flow,
		State: state,
		Steps: steps,
	}
	if state == core.RunWaitingApproval {
		status.ApprovalRequest = pending
	}
	if state == core.RunCompleted {
		status.Result = copyMap(envelope)
	}
	if state == core.RunFailed {
		for _, s := range steps {
			if s.Status == core.StepFailed && s.Error != "" {
				status.Error = s.Error
				break
			}
		}
	}
	return status
}

func deriveRunState(steps []core.StepResult, pending *core.ApprovalRequest) core.RunState {
	for _, s := range steps {
		if s.Status == core.StepFailed {
			return core.RunFailed
		}
	}
	if pending != nil {
		return core.RunWaitingApproval
	}
	for _, s := range steps {
		if s.Status != core.StepCompleted {
			return core.RunRunning
		}
	}
	return core.RunCompleted
}

func cloneWithDecision(envelope core.Envelope, decision core.ApprovalDecision) core.Envelope {
	out := copyMap(envelope)
	out["decision"] = map[string]any{
		"approved": decision.Approved,
		"comment":  decision.Comment,
		"by":       decision.By,
	}
	return out
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}

// Register registers RunFlow and ExecuteOperation with the Temporal worker.
func Register(w worker.Worker, wf *Workflows, acts *Activities) {
	w.RegisterWorkflowWithOptions(wf.RunFlow, workflow.RegisterOptions{Name: "RunFlow"})
	w.RegisterActivityWithOptions(acts.ExecuteOperation, activity.RegisterOptions{Name: ExecuteOperationName})
}
