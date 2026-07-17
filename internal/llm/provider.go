package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/ollama"
)

// NewChatModel returns a ChatModel selected by the LLM_PROVIDER environment
// variable. Defaults to Ollama for local development (no API key needed).
//
// Environment variables (Ollama):
//
//	LLM_PROVIDER    - "ollama" (default) or "claude"
//	OLLAMA_BASE_URL - Ollama server URL (default http://localhost:11434)
//	OLLAMA_MODEL    - Model to use (default llama3.2)
//
// Environment variables (Claude):
//
//	LLM_PROVIDER    - "claude"
//	ANTHROPIC_API_KEY - Anthropic API key (required)
//	CLAUDE_MODEL    - Model to use (default claude-sonnet-4-6)
func NewChatModel(ctx context.Context) (ChatModel, error) {
	provider := envOr("LLM_PROVIDER", "ollama")
	switch provider {
	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
			Model:   envOr("OLLAMA_MODEL", "gemma3:4b"),
			Timeout: 120 * time.Second,
			Format:  json.RawMessage(`"json"`),
		})
	case "claude":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY must be set when LLM_PROVIDER=claude")
		}
		return claude.NewChatModel(ctx, &claude.Config{
			APIKey:    apiKey,
			Model:     envOr("CLAUDE_MODEL", "claude-sonnet-4-6"),
			MaxTokens: 1024,
		})
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q; supported values: ollama, claude", provider)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
