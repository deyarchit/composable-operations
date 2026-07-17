package ops

import (
	"context"
	"fmt"
	"log/slog"

	"composable-operations/internal/core"
)

// remediateOp implements remediate: verifies the decision is approved, then
// logs the k8s scale call it would make in production. Swapping this op's
// implementation to issue a real PATCH to the configured endpoint is the
// only change needed to make remediation live.
type remediateOp struct{}

func (o *remediateOp) Type() string      { return "remediate" }
func (o *remediateOp) Kind() core.OpKind { return core.KindActivity }
func (o *remediateOp) ValidateParams(params map[string]any) error {
	for _, key := range []string{"deployment", "namespace", "replicas", "endpoint"} {
		if _, ok := params[key]; !ok {
			return fmt.Errorf("remediate: missing required param %q", key)
		}
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

	deployment, _ := params["deployment"].(string)
	namespace, _ := params["namespace"].(string)
	replicas, _ := params["replicas"].(int)
	endpoint, _ := params["endpoint"].(string)

	slog.Info("[DEMO] would PATCH k8s deployment to scale up — skipped in demo mode",
		"deployment", deployment,
		"namespace", namespace,
		"replicas", replicas,
		"endpoint", endpoint,
		"approved_by", decision["by"],
	)

	out := cloneEnvelope(input)
	out["remediated"] = true
	out["scaled_deployment"] = deployment
	out["scaled_to_replicas"] = replicas
	return out, nil
}
