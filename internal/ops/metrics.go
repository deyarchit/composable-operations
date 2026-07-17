package ops

import (
	"context"

	"composable-operations/internal/core"
)

// metricsCheckOp implements metrics.check: passes the envelope through unchanged.
// In production, replace with an implementation that fetches live metrics
// (Prometheus, Datadog, etc.) and merges them into the envelope.
// In the demo, metrics are supplied via the initial run input (sample.json).
type metricsCheckOp struct{}

func (o *metricsCheckOp) Type() string { return "metrics.check" }

func (o *metricsCheckOp) Kind() core.OpKind { return core.KindActivity }

func (o *metricsCheckOp) ValidateParams(_ map[string]any) error { return nil }

func (o *metricsCheckOp) Execute(_ context.Context, input core.Envelope, _ map[string]any) (core.Envelope, error) {
	return cloneEnvelope(input), nil
}
