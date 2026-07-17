package ops

import (
	"composable-operations/internal/core"
	"composable-operations/internal/llm"
	"composable-operations/internal/registry"
)

// RegisterBuiltins registers all built-in op types into reg. Both the API
// server (for definition validation) and the worker (for execution) call this
// at startup so both build from the same source of truth.
func RegisterBuiltins(reg *registry.Registry, client llm.Client) error {
	builtins := []core.Operation{
		&classifyOp{client: client},
		&piiOp{},
		&scoreOp{},
		&humanApprovalOp{},
		&decisionOp{client: client},
		&publishOp{},
	}
	for _, op := range builtins {
		if err := reg.Register(op); err != nil {
			return err
		}
	}
	return nil
}
