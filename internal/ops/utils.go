package ops

import (
	"bytes"
	"fmt"
	"maps"
	"text/template"

	"composable-operations/internal/core"
)

func cloneEnvelope(input core.Envelope) core.Envelope {
	out := make(core.Envelope, len(input))
	maps.Copy(out, input)
	return out
}

func renderTemplate(tmplStr string, data any) (string, error) {
	tmpl, err := template.New("").Option("missingkey=error").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
