package proxy

import "testing"

func TestMapModel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Existing short aliases
		{"deepseek-v4-pro", "deepseek/deepseek-v4-pro"},
		{"deepseek-v4", "deepseek/deepseek-v4-pro"},
		{"DEEPSEEK-PRO", "deepseek/deepseek-v4-pro"},
		{"deepseek-v4-flash", "deepseek/deepseek-v4-flash"},
		{"deepseek-flash", "deepseek/deepseek-v4-flash"},
		{"minimax-m2.7", "MiniMaxAI/MiniMax-M2.7"},
		{"minimax-m2.5", "MiniMaxAI/MiniMax-M2.5"},
		{"minimax", "MiniMaxAI/MiniMax-M2.5"},
		{"glm-5.1", "zai-org/GLM-5.1"},
		{"glm-5", "zai-org/GLM-5"},
		{"kimi-k2.6", "moonshotai/Kimi-K2.6"},
		{"kimi-k2.5", "moonshotai/Kimi-K2.5"},
		{"qwen-3.6-max-preview", "Qwen/Qwen3.6-Max-Preview"},
		{"qwen3.6", "Qwen/Qwen3.6-Plus"},
		{"step-3.5-flash", "stepfun/Step-3.5-Flash"},
		{"gemini-3.1-flash-lite", "google/gemini-3.1-flash-lite"},

		// New short aliases
		{"minimax-m3", "MiniMaxAI/MiniMax-M3"},
		{"minimax3", "MiniMaxAI/MiniMax-M3"},
		{"qwen-3.7-max-free", "Qwen/Qwen3.7-Max-Free"},
		{"qwen3.7-max-free", "Qwen/Qwen3.7-Max-Free"},
		{"qwen-3.7-max", "Qwen/Qwen3.7-Max"},
		{"qwen3.7-max", "Qwen/Qwen3.7-Max"},
		{"step-3.7-flash", "stepfun/Step-3.7-Flash"},
		{"step3.7", "stepfun/Step-3.7-Flash"},
		{"mimo-v2.5-pro", "xiaomi/mimo-v2.5-pro"},
		{"mimo-pro", "xiaomi/mimo-v2.5-pro"},
		{"mimo-v2.5", "xiaomi/mimo-v2.5"},
		{"mimo", "xiaomi/mimo-v2.5"},

		// New full IDs pass through
		{"MiniMaxAI/MiniMax-M3", "MiniMaxAI/MiniMax-M3"},
		{"Qwen/Qwen3.7-Max-Free", "Qwen/Qwen3.7-Max-Free"},
		{"Qwen/Qwen3.7-Max", "Qwen/Qwen3.7-Max"},
		{"stepfun/Step-3.7-Flash", "stepfun/Step-3.7-Flash"},
		{"xiaomi/mimo-v2.5-pro", "xiaomi/mimo-v2.5-pro"},
		{"xiaomi/mimo-v2.5", "xiaomi/mimo-v2.5"},

		// Unknown names pass through unchanged
		{"some/unknown-model", "some/unknown-model"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"", ""},
	}
	for _, c := range cases {
		got := MapModel(c.in)
		if got != c.want {
			t.Errorf("MapModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
