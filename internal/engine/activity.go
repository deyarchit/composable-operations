package engine

import (
	"context"
	"fmt"

	"composable-operations/internal/core"
	"composable-operations/internal/registry"
)

// ExecuteOperationName is the registered name for the ExecuteOperation activity.
const ExecuteOperationName = "ExecuteOperation"

// ActivityInput is the argument to ExecuteOperation.
type ActivityInput struct {
	Step     core.StepConfig
	Envelope core.Envelope
}

// ActivityOutput is the result from ExecuteOperation.
type ActivityOutput struct {
	Envelope core.Envelope
}

// Activities holds the registry needed to dispatch activity ops.
type Activities struct {
	Registry *registry.Registry
}

// ExecuteOperation is the single generic Temporal activity that backs all
// Activity-kind ops. It looks up the op by type, runs it, and returns the
// enriched envelope. One registration covers every deterministic and LLM op,
// so worker setup stays trivial as new op types are added.
func (a *Activities) ExecuteOperation(ctx context.Context, input ActivityInput) (ActivityOutput, error) {
	op, ok := a.Registry.Get(input.Step.Type)
	if !ok {
		return ActivityOutput{}, fmt.Errorf("execute: unknown op type %q", input.Step.Type)
	}

	actOp, ok := op.(core.ActivityOp)
	if !ok {
		return ActivityOutput{}, fmt.Errorf("execute: op %q is not an ActivityOp (kind=%s)", input.Step.Type, op.Kind())
	}

	result, err := actOp.Execute(ctx, input.Envelope, input.Step.Params)
	if err != nil {
		return ActivityOutput{}, fmt.Errorf("execute %q: %w", input.Step.Type, err)
	}

	return ActivityOutput{Envelope: result}, nil
}
