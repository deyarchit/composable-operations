package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"composable-operations/internal/core"
	"composable-operations/internal/llm"
)

// decisionOp implements llm.decision: calls the LLM with a rendered prompt and
// parses the response as an approve/reject decision. It emits the same
// decision{approved, comment, by} shape as human.approval, making it a
// drop-in replacement via a YAML-only change.
type decisionOp struct {
	client llm.Client
}

func (o *decisionOp) Type() string      { return "llm.decision" }
func (o *decisionOp) Kind() core.OpKind { return core.KindActivity }
func (o *decisionOp) ValidateParams(params map[string]any) error {
	if _, ok := params["prompt_template"]; !ok {
		return fmt.Errorf("llm.decision: missing required param 'prompt_template'")
	}
	return nil
}

func (o *decisionOp) Execute(ctx context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	tmplStr, _ := params["prompt_template"].(string)

	prompt, err := renderTemplate(tmplStr, input)
	if err != nil {
		return nil, fmt.Errorf("llm.decision: render template: %w", err)
	}

	completion, err := o.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm.decision: llm call: %w", err)
	}

	var parsed struct {
		Approved bool   `json:"approved"`
		Comment  string `json:"comment"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(completion.Text)), &parsed); err != nil {
		return nil, fmt.Errorf("llm.decision: parse response %q: %w", completion.Text, err)
	}

	out := cloneEnvelope(input)
	out["decision"] = map[string]any{
		"approved": parsed.Approved,
		"comment":  parsed.Comment,
		"by":       "llm",
	}
	return out, nil
}
