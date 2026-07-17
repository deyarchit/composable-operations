package ops

import (
	"context"
	"fmt"

	"composable-operations/internal/core"
)

// logsCheckOp implements logs.check: places fixture log lines into the envelope
// under the "logs" key. In production, replace with a real log source (Loki,
// Elasticsearch, CloudWatch, etc.) by swapping this op's implementation.
type logsCheckOp struct{}

func (o *logsCheckOp) Type() string      { return "logs.check" }
func (o *logsCheckOp) Kind() core.OpKind { return core.KindActivity }
func (o *logsCheckOp) ValidateParams(params map[string]any) error {
	fixture, ok := params["fixture"].([]any)
	if !ok || len(fixture) == 0 {
		return fmt.Errorf("logs.check: 'fixture' must be a non-empty list of strings")
	}
	return nil
}

func (o *logsCheckOp) Execute(_ context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	fixture, _ := params["fixture"].([]any)
	out := cloneEnvelope(input)
	out["logs"] = fixture
	return out, nil
}
