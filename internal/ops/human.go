package ops

import (
	"fmt"
	"maps"

	"composable-operations/internal/core"
)

// humanApprovalOp implements human.approval: a HumanGate that suspends the run
// and produces an ApprovalRequest for the reviewer. The workflow handles the
// signal wait and decision recording; this op only constructs the request.
type humanApprovalOp struct{}

func (o *humanApprovalOp) Type() string      { return "human.approval" }
func (o *humanApprovalOp) Kind() core.OpKind { return core.KindHumanGate }
func (o *humanApprovalOp) ValidateParams(params map[string]any) error {
	if _, ok := params["prompt"]; !ok {
		return fmt.Errorf("human.approval: missing required param 'prompt'")
	}
	return nil
}

func (o *humanApprovalOp) BuildRequest(input core.Envelope, params map[string]any) core.ApprovalRequest {
	prompt, _ := params["prompt"].(string)

	payload := buildPayload(input, params)

	return core.ApprovalRequest{
		Prompt:  prompt,
		Payload: payload,
	}
}

// buildPayload selects envelope fields using display_fields param; if absent,
// returns the whole envelope.
func buildPayload(input core.Envelope, params map[string]any) map[string]any {
	rawFields, ok := params["display_fields"]
	if !ok {
		payload := make(map[string]any, len(input))
		maps.Copy(payload, input)
		return payload
	}

	fields, ok := rawFields.([]any)
	if !ok {
		payload := make(map[string]any, len(input))
		maps.Copy(payload, input)
		return payload
	}

	payload := make(map[string]any, len(fields))
	for _, f := range fields {
		if key, ok := f.(string); ok {
			if val, exists := input[key]; exists {
				payload[key] = val
			}
		}
	}
	return payload
}
