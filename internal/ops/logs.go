package ops

import (
	"context"

	"composable-operations/internal/core"
)

// logsCheckOp implements logs.check: passes the envelope through unchanged.
// In production, replace with an implementation that fetches recent log lines
// (Loki, Elasticsearch, CloudWatch, etc.) and sets the "logs" key.
// In the demo, logs are supplied via the initial run input (sample.json).
type logsCheckOp struct{}

func (o *logsCheckOp) Type() string { return "logs.check" }

func (o *logsCheckOp) Kind() core.OpKind { return core.KindActivity }

func (o *logsCheckOp) ValidateParams(_ map[string]any) error { return nil }

func (o *logsCheckOp) Execute(_ context.Context, input core.Envelope, _ map[string]any) (core.Envelope, error) {
	return cloneEnvelope(input), nil
}
