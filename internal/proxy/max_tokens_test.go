package proxy

import (
	"testing"

	"github.com/dev2k6/command-code-proxy-server/internal/api"
)

func TestMaxTokensClampedToAPIlimit(t *testing.T) {
	p := NewProxy("")

	tests := []struct {
		name       string
		maxTokens  *int
		maxCompTok *int
		want       int
	}{
		{"default when unset", nil, nil, 64000},
		{"small value passes through", intPtr(1000), nil, 1000},
		{"exactly at limit passes", intPtr(200000), nil, 200000},
		{"huge value clamped", intPtr(1000000), nil, 200000},
		{"huge max_completion_tokens clamped", nil, intPtr(1000000), 200000},
		{"max_completion_tokens wins over max_tokens", intPtr(5000), intPtr(999999), 200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := api.OpenAIChatRequest{
				Model:               "deepseek-v4-flash",
				Messages:            []api.OpenAIMessage{{Role: "user", Content: "hi"}},
				MaxTokens:           tt.maxTokens,
				MaxCompletionTokens: tt.maxCompTok,
			}
			ccBody, err := p.BuildRequest(req)
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}
			if ccBody.Params.MaxTokens != tt.want {
				t.Fatalf("expected max_tokens=%d, got %d", tt.want, ccBody.Params.MaxTokens)
			}
		})
	}
}

func intPtr(v int) *int {
	return &v
}
