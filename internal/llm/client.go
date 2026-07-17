package llm

import "github.com/cloudwego/eino/components/model"

// ChatModel is the seam all LLM ops call through. It aliases eino's
// BaseChatModel so concrete providers (Anthropic, OpenAI, Ollama, etc.) can be
// wired in via the cloudwego/eino-ext integrations without changing op code.
type ChatModel = model.BaseChatModel
