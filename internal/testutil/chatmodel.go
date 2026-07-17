package testutil

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// StubChatModel is a deterministic ChatModel for use in tests. It inspects the
// last user message for keywords and returns canned JSON responses matching
// the incident-response flow, so tests run end-to-end without a real LLM.
//
// Matching priority (highest first):
//  1. Decision prompts: detected by `"approved"` literal in the JSON schema
//  2. Analysis prompts: detected by "root_cause" or "analyze" keyword
type StubChatModel struct{}

func (s *StubChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	lower := strings.ToLower(lastUserContent(input))

	var text string
	switch {
	case strings.Contains(lower, `"approved"`) || strings.Contains(lower, "respond with json") && strings.Contains(lower, "approved"):
		text = `{"approved":true,"comment":"Analysis confirms remediation is warranted."}`
	case strings.Contains(lower, "root_cause") || strings.Contains(lower, "root cause") || strings.Contains(lower, "analyze"):
		text = `{"root_cause":"database connection pool exhausted","severity":"critical","recommended_action":"scale-db-connections and restart payment-api"}`
	default:
		text = `{"approved":true,"comment":"ok"}`
	}
	return schema.AssistantMessage(text, nil), nil
}

func (s *StubChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := s.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func lastUserContent(msgs []*schema.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schema.User {
			return msgs[i].Content
		}
	}
	if len(msgs) > 0 {
		return msgs[len(msgs)-1].Content
	}
	return ""
}
