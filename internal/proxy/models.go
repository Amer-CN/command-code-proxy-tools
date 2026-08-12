package proxy

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/dev2k6/command-code-proxy-server/internal/api"
)

// modelInfo 描述一个模型及其所属套餐。
// Go 套餐字段与官方 https://commandcode.ai/docs/plans/go 的 32 个模型表保持一致。
type modelInfo struct {
	ID      string
	OwnedBy string
	OnGo    bool // 是否包含在 Go 套餐（32 个）内
}

// catalogModels 官方完整模型目录（55 个），按厂商 A→Z 分组。
// 与 https://commandcode.ai/docs/reference/cli/models 保持一致。
var catalogModels = []modelInfo{
	// Alibaba (Qwen)
	{ID: "Qwen/Qwen3.6-Max-Preview", OwnedBy: "alibaba", OnGo: true},
	{ID: "Qwen/Qwen3.6-Plus", OwnedBy: "alibaba", OnGo: true},
	{ID: "Qwen/Qwen3.7-Flash", OwnedBy: "alibaba", OnGo: true},
	{ID: "Qwen/Qwen3.7-Max", OwnedBy: "alibaba", OnGo: true},
	{ID: "Qwen/Qwen3.7-Plus", OwnedBy: "alibaba", OnGo: true},
	{ID: "Qwen/Qwen3.8-Max", OwnedBy: "alibaba", OnGo: true},
	// Anthropic —— Go 套餐不含（需 Pro/Max）
	{ID: "claude-fable-5", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-haiku-4-5", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-opus-4-6", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-opus-4-7", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-opus-4-8", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-opus-5", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-sonnet-4-5", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-sonnet-4-6", OwnedBy: "anthropic", OnGo: false},
	{ID: "claude-sonnet-5", OwnedBy: "anthropic", OnGo: false},
	// DeepSeek
	{ID: "deepseek/deepseek-v4-flash", OwnedBy: "deepseek", OnGo: true},
	{ID: "deepseek/deepseek-v4-pro", OwnedBy: "deepseek", OnGo: true},
	// Google —— Go 套餐不含（需 Pro/Max）
	{ID: "google/gemini-3.1-flash-lite", OwnedBy: "google", OnGo: false},
	{ID: "google/gemini-3.5-flash", OwnedBy: "google", OnGo: false},
	{ID: "google/gemini-3.5-flash-lite", OwnedBy: "google", OnGo: false},
	{ID: "google/gemini-3.6-flash", OwnedBy: "google", OnGo: false},
	// InclusionAI —— 全量免费模型，不在 Go 套餐 32 个表内
	{ID: "inclusionai/ling-3.0-flash-free", OwnedBy: "inclusionai", OnGo: false},
	// Meta (Muse Spark)
	{ID: "meta/muse-spark-1.1", OwnedBy: "meta", OnGo: false},
	{ID: "meta/muse-spark-1.2", OwnedBy: "meta", OnGo: false},
	{ID: "meta/muse-spark-1.2-contributor", OwnedBy: "meta", OnGo: true},
	// MiniMax
	{ID: "MiniMaxAI/MiniMax-M2.5", OwnedBy: "minimaxai", OnGo: true},
	{ID: "MiniMaxAI/MiniMax-M2.7", OwnedBy: "minimaxai", OnGo: true},
	{ID: "MiniMaxAI/MiniMax-M3", OwnedBy: "minimaxai", OnGo: true},
	// Moonshot AI
	{ID: "moonshotai/Kimi-K2.5", OwnedBy: "moonshotai", OnGo: true},
	{ID: "moonshotai/Kimi-K2.6", OwnedBy: "moonshotai", OnGo: true},
	{ID: "moonshotai/Kimi-K2.7-Code", OwnedBy: "moonshotai", OnGo: true},
	{ID: "moonshotai/Kimi-K2.7-Code-Highspeed", OwnedBy: "moonshotai", OnGo: true},
	{ID: "moonshotai/Kimi-K3", OwnedBy: "moonshotai", OnGo: true},
	// NVIDIA
	{ID: "nvidia/nemotron-3-ultra-550b-a55b", OwnedBy: "nvidia", OnGo: true},
	// OpenAI —— Go 只含 Luna，其余需 Pro/Max
	{ID: "gpt-5.3-codex", OwnedBy: "openai", OnGo: false},
	{ID: "gpt-5.4", OwnedBy: "openai", OnGo: false},
	{ID: "gpt-5.4-mini", OwnedBy: "openai", OnGo: false},
	{ID: "gpt-5.5", OwnedBy: "openai", OnGo: false},
	{ID: "gpt-5.6-luna", OwnedBy: "openai", OnGo: true},
	{ID: "gpt-5.6-sol", OwnedBy: "openai", OnGo: false},
	{ID: "gpt-5.6-terra", OwnedBy: "openai", OnGo: false},
	// Poolside
	{ID: "poolside/laguna-s-2.1-free", OwnedBy: "poolside", OnGo: true},
	// Sakana AI
	{ID: "sakana/fugu-ultra", OwnedBy: "sakana", OnGo: false},
	// StepFun
	{ID: "stepfun/Step-3.5-Flash", OwnedBy: "stepfun", OnGo: true},
	{ID: "stepfun/Step-3.7-Flash", OwnedBy: "stepfun", OnGo: true},
	// Tencent
	{ID: "tencent/hy3-paid", OwnedBy: "tencent", OnGo: true},
	// Thinking Machines
	{ID: "thinkingmachines/inkling", OwnedBy: "thinkingmachines", OnGo: true},
	{ID: "thinkingmachines/inkling-small", OwnedBy: "thinkingmachines", OnGo: true},
	// xAI
	{ID: "xai/grok-4.5", OwnedBy: "xai", OnGo: true},
	// Xiaomi
	{ID: "xiaomi/mimo-v2.5", OwnedBy: "xiaomi", OnGo: true},
	{ID: "xiaomi/mimo-v2.5-pro", OwnedBy: "xiaomi", OnGo: true},
	// Z AI
	{ID: "zai-org/GLM-5", OwnedBy: "zai-org", OnGo: true},
	{ID: "zai-org/GLM-5.1", OwnedBy: "zai-org", OnGo: true},
	{ID: "zai-org/GLM-5.2", OwnedBy: "zai-org", OnGo: true},
	{ID: "zai-org/GLM-5.2-Fast", OwnedBy: "zai-org", OnGo: true},
}

// HandleModels handles the /v1/models endpoint.
// 支持 ?plan=go 只返回 Go 套餐内模型（32 个），默认返回全部 55 个——与官方目录一致。
func (p *Proxy) HandleModels(w http.ResponseWriter, r *http.Request) {
	plan := r.URL.Query().Get("plan")
	items := catalogModels
	if plan == "go" {
		items = make([]modelInfo, 0, 32)
		for _, m := range catalogModels {
			if m.OnGo {
				items = append(items, m)
			}
		}
	}
	// 按厂商分组排序，保持目录整洁
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OwnedBy != items[j].OwnedBy {
			return items[i].OwnedBy < items[j].OwnedBy
		}
		return items[i].ID < items[j].ID
	})

	models := api.OpenAIModelList{
		Object: "list",
		Data:   make([]api.OpenAIModel, 0, len(items)),
	}
	for _, m := range items {
		models.Data = append(models.Data, api.OpenAIModel{
			ID:      m.ID,
			Object:  "model",
			Created: 0,
			OwnedBy: m.OwnedBy,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}