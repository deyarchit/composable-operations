package ops

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"text/template"

	"composable-operations/internal/core"
	"composable-operations/internal/llm"
)

// classifyOp implements llm.classify: renders a prompt template, calls the
// LLM, and writes the response as a numeric score into output_field.
type classifyOp struct {
	client llm.Client
}

func (o *classifyOp) Type() string      { return "llm.classify" }
func (o *classifyOp) Kind() core.OpKind { return core.KindActivity }
func (o *classifyOp) ValidateParams(params map[string]any) error {
	if _, ok := params["prompt_template"]; !ok {
		return fmt.Errorf("llm.classify: missing required param 'prompt_template'")
	}
	if _, ok := params["output_field"]; !ok {
		return fmt.Errorf("llm.classify: missing required param 'output_field'")
	}
	return nil
}

func (o *classifyOp) Execute(ctx context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	tmplStr, _ := params["prompt_template"].(string)
	outputField, _ := params["output_field"].(string)

	prompt, err := renderTemplate(tmplStr, input)
	if err != nil {
		return nil, fmt.Errorf("llm.classify: render template: %w", err)
	}

	completion, err := o.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm.classify: llm call: %w", err)
	}

	score, err := strconv.ParseFloat(strings.TrimSpace(completion.Text), 64)
	if err != nil {
		return nil, fmt.Errorf("llm.classify: parse score %q: %w", completion.Text, err)
	}

	out := cloneEnvelope(input)
	out[outputField] = score
	return out, nil
}

func renderTemplate(tmplStr string, data any) (string, error) {
	tmpl, err := template.New("").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func cloneEnvelope(e core.Envelope) core.Envelope {
	out := make(core.Envelope, len(e))
	maps.Copy(out, e)
	return out
}
