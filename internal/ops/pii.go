package ops

import (
	"context"
	"fmt"
	"regexp"

	"composable-operations/internal/core"
)

// piiOp implements pii.scan: searches the content field for regex patterns and
// sets pii_found to true when the match count meets or exceeds threshold.
type piiOp struct{}

func (o *piiOp) Type() string      { return "pii.scan" }
func (o *piiOp) Kind() core.OpKind { return core.KindActivity }
func (o *piiOp) ValidateParams(params map[string]any) error {
	if _, ok := params["patterns"]; !ok {
		return fmt.Errorf("pii.scan: missing required param 'patterns'")
	}
	return nil
}

func (o *piiOp) Execute(_ context.Context, input core.Envelope, params map[string]any) (core.Envelope, error) {
	content, _ := input["content"].(string)

	patterns, err := extractPatterns(params)
	if err != nil {
		return nil, fmt.Errorf("pii.scan: %w", err)
	}

	threshold := 1
	if t, ok := params["threshold"]; ok {
		switch v := t.(type) {
		case int:
			threshold = v
		case float64:
			threshold = int(v)
		}
	}

	matches := 0
	for _, re := range patterns {
		if re.MatchString(content) {
			matches++
		}
	}

	out := cloneEnvelope(input)
	out["pii_found"] = matches >= threshold
	return out, nil
}

func extractPatterns(params map[string]any) ([]*regexp.Regexp, error) {
	rawPatterns, ok := params["patterns"]
	if !ok {
		return nil, fmt.Errorf("missing 'patterns'")
	}
	list, ok := rawPatterns.([]any)
	if !ok {
		return nil, fmt.Errorf("'patterns' must be a list of strings")
	}
	patterns := make([]*regexp.Regexp, 0, len(list))
	for _, p := range list {
		s, ok := p.(string)
		if !ok {
			return nil, fmt.Errorf("each pattern must be a string")
		}
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", s, err)
		}
		patterns = append(patterns, re)
	}
	return patterns, nil
}
