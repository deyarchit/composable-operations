package ops

import (
	"context"
	"fmt"
	"log/slog"

	"composable-operations/internal/core"
)

// publishOp implements publish: sets published=true on the envelope when
// decision.approved is true. Rejects publication otherwise so an unapproved
// payload can never pass through even if the flow is misconfigured.
type publishOp struct{}

func (o *publishOp) Type() string      { return "publish" }
func (o *publishOp) Kind() core.OpKind { return core.KindActivity }
func (o *publishOp) ValidateParams(_ map[string]any) error {
	return nil
}

func (o *publishOp) Execute(_ context.Context, input core.Envelope, _ map[string]any) (core.Envelope, error) {
	decision, ok := input["decision"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("publish: 'decision' field missing or malformed in envelope")
	}

	approved, _ := decision["approved"].(bool)
	if !approved {
		return nil, fmt.Errorf("publish: content not approved for publication")
	}

	slog.Info("Publishing content", "by", decision["by"])

	out := cloneEnvelope(input)
	out["published"] = true
	return out, nil
}
