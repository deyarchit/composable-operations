package llm

import (
	"context"
	"strings"
)

// StubClient is a deterministic LLMClient for demo and test use. It inspects
// the prompt for keywords and returns a canned response appropriate for the
// moderation flow, so the demo runs end-to-end without a real LLM provider.
type StubClient struct{}

func (s *StubClient) Complete(_ context.Context, prompt string) (Completion, error) {
	lower := strings.ToLower(prompt)

	switch {
	case strings.Contains(lower, "toxicity"):
		return Completion{Text: "0.15"}, nil
	case strings.Contains(lower, "policy"):
		return Completion{Text: "0.08"}, nil
	case strings.Contains(lower, "approve") || strings.Contains(lower, "decision") || strings.Contains(lower, "risk"):
		return Completion{Text: `{"approved":true,"comment":"Content meets policy standards."}`}, nil
	default:
		return Completion{Text: "0.1"}, nil
	}
}
