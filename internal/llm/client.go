package llm

import "context"

// Completion is the result of a single LLM call.
type Completion struct {
	Text string
}

// Client is the seam all LLM ops call through. The concrete provider
// (Anthropic, OpenAI, etc.) is chosen at startup and injected; ops never
// reference a provider directly.
type Client interface {
	Complete(ctx context.Context, prompt string) (Completion, error)
}
