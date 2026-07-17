package ops

import (
	"context"
	"fmt"

	"composable-operations/internal/core"
)

// scoreOp implements score: computes a weighted sum of numeric envelope fields
// and writes it to risk_score.
type scoreOp struct{}

func (o *scoreOp) Type() string      { return "score" }
func (o *scoreOp) Kind() core.OpKind { return core.KindActivity }
func (o *scoreOp) ValidateParams(params map[string]any) error {
	if _, ok := params["weights"]; !ok {
		return fmt.Errorf("score: missing required param 'weights'")
	}
	return nil
}

func (o *scoreOp) Execute(_ context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	weightsRaw, _ := params["weights"].(map[string]any)

	var riskScore float64
	for field, weightRaw := range weightsRaw {
		weight, ok := toFloat64(weightRaw)
		if !ok {
			return nil, fmt.Errorf("score: weight for %q must be numeric", field)
		}
		value, _ := toFloat64(input[field])
		riskScore += value * weight
	}

	out := cloneEnvelope(input)
	out["risk_score"] = riskScore
	return out, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
