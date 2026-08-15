package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev2k6/command-code-proxy-server/internal/api"
)

// TestHandleModels_FullCatalog 断言 /v1/models 返回官方完整模型目录（55 个）。
func TestHandleModels_FullCatalog(t *testing.T) {
	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	p.HandleModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var list api.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(list.Data) != 55 {
		t.Errorf("model count = %d, want 55", len(list.Data))
	}

	// 关键模型抽查：官方目录里新增的模型必须都在
	want := []string{
		"Qwen/Qwen3.8-Max",
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-fable-5",
		"claude-haiku-4-5",
		"claude-opus-4-6",
		"claude-sonnet-4-5",
		"google/gemini-3.5-flash",
		"google/gemini-3.6-flash",
		"inclusionai/ling-3.0-flash-free",
		"meta/muse-spark-1.1",
		"meta/muse-spark-1.2",
		"meta/muse-spark-1.2-contributor",
		"moonshotai/Kimi-K3",
		"moonshotai/Kimi-K2.7-Code",
		"moonshotai/Kimi-K2.7-Code-Highspeed",
		"nvidia/nemotron-3-ultra-550b-a55b",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.5",
		"gpt-5.6-luna",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"poolside/laguna-s-2.1-free",
		"sakana/fugu-ultra",
		"tencent/hy3-paid",
		"thinkingmachines/inkling",
		"thinkingmachines/inkling-small",
		"xai/grok-4.5",
		"zai-org/GLM-5.2",
		"zai-org/GLM-5.2-Fast",
		// 原有 18 个里的关键模型仍然保留
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro",
		"MiniMaxAI/MiniMax-M3",
	}
	got := make(map[string]bool, len(list.Data))
	for _, m := range list.Data {
		got[m.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing model %q", id)
		}
	}

	// 不允许重复 ID
	seen := make(map[string]bool, len(list.Data))
	for _, m := range list.Data {
		if seen[m.ID] {
			t.Errorf("duplicate model id %q", m.ID)
		}
		seen[m.ID] = true
	}
}

// TestHandleModels_GoPlan 断言 ?plan=go 只返回 Go 套餐内的 32 个模型。
func TestHandleModels_GoPlan(t *testing.T) {
	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/v1/models?plan=go", nil)
	rec := httptest.NewRecorder()
	p.HandleModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var list api.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(list.Data) != 32 {
		t.Errorf("go plan model count = %d, want 32", len(list.Data))
	}

	// Go 套餐必须包含的模型
	want := []string{
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro",
		"Qwen/Qwen3.8-Max",
		"Qwen/Qwen3.7-Max",
		"Qwen/Qwen3.7-Plus",
		"Qwen/Qwen3.7-Flash",
		"moonshotai/Kimi-K3",
		"moonshotai/Kimi-K2.7-Code",
		"zai-org/GLM-5.2",
		"zai-org/GLM-5.2-Fast",
		"MiniMaxAI/MiniMax-M3",
		"gpt-5.6-luna",
		"poolside/laguna-s-2.1-free",
		"tencent/hy3-paid",
		"xai/grok-4.5",
		"xiaomi/mimo-v2.5-pro",
		"stepfun/Step-3.7-Flash",
		"nvidia/nemotron-3-ultra-550b-a55b",
		"meta/muse-spark-1.2-contributor",
	}
	got := make(map[string]bool, len(list.Data))
	for _, m := range list.Data {
		got[m.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("go plan missing model %q", id)
		}
	}

	// Go 套餐绝不应包含的模型（需 Pro/Max）
	notWant := []string{
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-fable-5",
		"google/gemini-3.6-flash",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.5",
		"sakana/fugu-ultra",
		"meta/muse-spark-1.2",
	}
	for _, id := range notWant {
		if got[id] {
			t.Errorf("go plan should NOT include %q", id)
		}
	}
}