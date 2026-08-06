package proxy

import (
	"testing"

	"github.com/dev2k6/command-code-proxy-server/internal/api"
)

func TestConvertMessagesSkipsEmptyContent(t *testing.T) {
	msgs := []api.OpenAIMessage{
		{Role: "user", Content: "hello"},
		{Role: "user", Content: nil},     // null content (e.g. reasoning item from Codex)
		{Role: "user", Content: ""},      // empty string content
		{Role: "user", Content: []any{}}, // empty content array
		{Role: "assistant", Content: "reply"},
		{Role: "assistant", ToolCalls: []api.ToolCall{{ID: "c1", Type: "function", Function: api.FunctionCall{Name: "bash", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "c1", Name: "bash", Content: "ok"},
	}

	cc := ConvertMessages(msgs)
	if len(cc) != 4 {
		t.Fatalf("expected 4 messages (3 empty skipped), got %d", len(cc))
	}
	for i, m := range cc {
		if m.Content == nil || len(m.Content) == 0 {
			t.Fatalf("message %d (role %s) has empty content", i, m.Role)
		}
	}
	if cc[0].Role != "user" || cc[1].Role != "assistant" || cc[2].Role != "assistant" || cc[3].Role != "tool" {
		t.Fatalf("unexpected order/roles: %+v", cc)
	}
}
