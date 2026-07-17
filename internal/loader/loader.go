package loader

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"composable-operations/internal/core"
	"composable-operations/internal/registry"
)

// Loader reads flow definitions from a directory of YAML files and validates
// them against a Registry before returning. It is the single place that touches
// disk; swapping to DB-backed definitions means changing only this package.
type Loader struct {
	dir      string
	registry *registry.Registry
}

// New returns a Loader that reads from dir and validates against reg.
func New(dir string, reg *registry.Registry) *Loader {
	return &Loader{dir: dir, registry: reg}
}

// Load parses flows/{name}.yaml and returns a validated FlowDefinition.
// Validation fails fast: unknown types and duplicate step IDs are caught
// before any run starts, so a bad definition can never enter Temporal history.
func (l *Loader) Load(name string) (*core.FlowDefinition, error) {
	path := filepath.Join(l.dir, name+".yaml")
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from a controlled name, not user input
	if err != nil {
		return nil, fmt.Errorf("loader: read %s: %w", path, err)
	}

	var def core.FlowDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("loader: parse %s: %w", path, err)
	}

	if err := l.validate(&def); err != nil {
		return nil, fmt.Errorf("loader: validate %s: %w", name, err)
	}

	return &def, nil
}

func (l *Loader) validate(def *core.FlowDefinition) error {
	if len(def.Steps) == 0 {
		return fmt.Errorf("flow has no steps")
	}

	seen := make(map[string]bool, len(def.Steps))
	for i, step := range def.Steps {
		if step.ID == "" {
			return fmt.Errorf("step[%d] has no id", i)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = true

		op, ok := l.registry.Get(step.Type)
		if !ok {
			return fmt.Errorf("step %q references unknown op type %q", step.ID, step.Type)
		}

		if err := op.ValidateParams(step.Params); err != nil {
			return fmt.Errorf("step %q invalid params: %w", step.ID, err)
		}
	}
	return nil
}
