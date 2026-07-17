package ops

import (
	"context"
	"fmt"
	"maps"

	"composable-operations/internal/core"
)

// metricsCheckOp implements metrics.check: copies fixture metrics from params
// into the envelope. In production, replace with a real metrics source (Prometheus,
// Datadog, etc.) by swapping this op's implementation.
type metricsCheckOp struct{}

func (o *metricsCheckOp) Type() string      { return "metrics.check" }
func (o *metricsCheckOp) Kind() core.OpKind { return core.KindActivity }
func (o *metricsCheckOp) ValidateParams(params map[string]any) error {
	fixture, ok := params["fixture"].(map[string]any)
	if !ok || len(fixture) == 0 {
		return fmt.Errorf("metrics.check: 'fixture' must be a non-empty map")
	}
	return nil
}

func (o *metricsCheckOp) Execute(_ context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	fixture, _ := params["fixture"].(map[string]any)
	out := cloneEnvelope(input)
	maps.Copy(out, fixture)
	return out, nil
}
