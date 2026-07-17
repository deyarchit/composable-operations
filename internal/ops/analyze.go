package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"composable-operations/internal/core"
	"composable-operations/internal/llm"
)

// analyzeOp implements llm.analyze: renders a prompt from the envelope, calls
// the LLM, and parses the response as a structured analysis map written to the
// "analysis" key. Used to identify root cause, severity, and recommended action.
type analyzeOp struct {
	client llm.ChatModel
}

func (o *analyzeOp) Type() string      { return "llm.analyze" }
func (o *analyzeOp) Kind() core.OpKind { return core.KindActivity }
func (o *analyzeOp) ValidateParams(params map[string]any) error {
	if _, ok := params["prompt_template"]; !ok {
		return fmt.Errorf("llm.analyze: missing required param 'prompt_template'")
	}
	return nil
}

func (o *analyzeOp) Execute(ctx context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	tmplStr, _ := params["prompt_template"].(string)

	prompt, err := renderTemplate(tmplStr, input)
	if err != nil {
		return nil, fmt.Errorf("llm.analyze: render template: %w", err)
	}

	msg, err := o.client.Generate(ctx, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		return nil, fmt.Errorf("llm.analyze: llm call: %w", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(msg.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("llm.analyze: parse response %q: %w", msg.Content, err)
	}

	out := cloneEnvelope(input)
	out["analysis"] = parsed
	return out, nil
}
