package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
)

// NewChatModel returns a ChatModel selected by the LLM_PROVIDER environment
// variable. Defaults to Ollama for local development (no API key needed).
//
// Environment variables:
//
//	LLM_PROVIDER   - "ollama" (default) or future providers
//	OLLAMA_BASE_URL - Ollama server URL (default http://localhost:11434)
//	OLLAMA_MODEL    - Model to use (default llama3.2)
func NewChatModel(ctx context.Context) (ChatModel, error) {
	provider := envOr("LLM_PROVIDER", "ollama")
	switch provider {
	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
			Model:   envOr("OLLAMA_MODEL", "llama3.2"),
			Timeout: 120 * time.Second,
		})
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER %q; supported values: ollama", provider)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
