package registry

import (
	"fmt"

	"composable-operations/internal/core"
)

// Registry is a code-level catalog mapping op type names to implementations.
// It is a shared library imported by both the API server (for validation) and
// the worker (for execution); each binary populates it with RegisterBuiltins.
type Registry struct {
	ops map[string]core.Operation
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{ops: make(map[string]core.Operation)}
}

// Register adds op to the registry. Returns an error if the type name is
// already registered, so duplicate registrations surface at startup.
func (r *Registry) Register(op core.Operation) error {
	name := op.Type()
	if _, exists := r.ops[name]; exists {
		return fmt.Errorf("op type %q already registered", name)
	}
	r.ops[name] = op
	return nil
}

// Get returns the Operation for typeName, or false if not registered.
func (r *Registry) Get(typeName string) (core.Operation, bool) {
	op, ok := r.ops[typeName]
	return op, ok
}

// Has reports whether typeName is registered.
func (r *Registry) Has(typeName string) bool {
	_, ok := r.ops[typeName]
	return ok
}
