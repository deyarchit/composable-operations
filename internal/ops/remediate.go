package ops

import (
	"context"
	"fmt"
	"log/slog"

	"composable-operations/internal/core"
)

// remediateOp implements remediate: verifies the decision is approved, then
// logs the remediation call it would make in production. Swapping this op's
// implementation to issue a real HTTP POST to the configured endpoint is the
// only change needed to make remediation live.
type remediateOp struct{}

func (o *remediateOp) Type() string      { return "remediate" }
func (o *remediateOp) Kind() core.OpKind { return core.KindActivity }
func (o *remediateOp) ValidateParams(params map[string]any) error {
	if _, ok := params["endpoint"]; !ok {
		return fmt.Errorf("remediate: missing required param 'endpoint'")
	}
	return nil
}

func (o *remediateOp) Execute(_ context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	decision, _ := input["decision"].(map[string]any)
	if decision == nil {
		return nil, fmt.Errorf("remediate: missing 'decision' in envelope")
	}
	if approved, _ := decision["approved"].(bool); !approved {
		comment, _ := decision["comment"].(string)
		return nil, fmt.Errorf("remediate: not approved: %s", comment)
	}

	endpoint, _ := params["endpoint"].(string)

	var action string
	if analysis, ok := input["analysis"].(map[string]any); ok {
		action, _ = analysis["recommended_action"].(string)
	}

	slog.Info("[DEMO] remediation triggered — would POST to endpoint in production",
		"endpoint", endpoint,
		"action", action,
		"approved_by", decision["by"],
	)

	out := cloneEnvelope(input)
	out["remediated"] = true
	return out, nil
}
