package proxy

import "strings"

// normalizedModelID 把客户端传入的模型名映射为 CommandCode 官方完整模型 ID。
// 规则（与官方 https://commandcode.ai/docs/reference/cli/models 一致）：
//   - 完整 ID（如 "moonshotai/Kimi-K3"）原样透传；
//   - 短名（如 "kimi-k3"）通过别名表解析 —— 大小写不敏感、忽略厂商前缀与分隔符；
//   - 未知名称原样透传（由上游 CommandCode 决定是否接受）。
func MapModel(name string) string {
	key := normalizeAlias(name)
	if v, ok := modelAliases[key]; ok {
		return v
	}
	return name // pass through as-is
}

// normalizeAlias 把任意输入归一化成别名键：小写、去厂商前缀、去非字母数字。
func normalizeAlias(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	// 去掉厂商前缀（"deepseek/..."、"moonshotai/..." 等）
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	// 去掉所有非字母数字字符（-、.、_、空格等）
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// modelAliases 短名 → 官方完整 ID 别名表（覆盖全部 55 个模型）。
// 键为 normalizeAlias 的输出；每个模型至少一个别名，常见变体尽量收录。
var modelAliases = map[string]string{
	// Alibaba (Qwen)
	"qwen36maxpreview": "Qwen/Qwen3.6-Max-Preview",
	"qwen36plus":       "Qwen/Qwen3.6-Plus",
	"qwen36":           "Qwen/Qwen3.6-Plus",
	"qwen37flash":      "Qwen/Qwen3.7-Flash",
	"qwen37max":        "Qwen/Qwen3.7-Max",
	"qwen37maxfree":    "Qwen/Qwen3.7-Max-Free", // 已下架，保留旧别名兼容
	"qwen37plus":       "Qwen/Qwen3.7-Plus",
	"qwen38max":        "Qwen/Qwen3.8-Max",
	// Anthropic —— 官方 ID 无厂商前缀
	"claudefable5":      "claude-fable-5",
	"fable5":            "claude-fable-5",
	"claudehaiku45":     "claude-haiku-4-5",
	"haiku45":           "claude-haiku-4-5",
	"claudeopus46":      "claude-opus-4-6",
	"opus46":            "claude-opus-4-6",
	"claudeopus47":      "claude-opus-4-7",
	"opus47":            "claude-opus-4-7",
	"claudeopus48":      "claude-opus-4-8",
	"opus48":            "claude-opus-4-8",
	"claudeopus5":       "claude-opus-5",
	"opus5":             "claude-opus-5",
	"opus":              "claude-opus-5", // 含糊短名 → 最新 Opus
	"claudesonnet45":    "claude-sonnet-4-5",
	"sonnet45":          "claude-sonnet-4-5",
	"claudesonnet46":    "claude-sonnet-4-6",
	"sonnet46":          "claude-sonnet-4-6",
	"claudesonnet5":     "claude-sonnet-5",
	"sonnet5":           "claude-sonnet-5",
	"sonnet":            "claude-sonnet-5", // 含糊短名 → 最新 Sonnet
	// DeepSeek
	"deepseekv4flash": "deepseek/deepseek-v4-flash",
	"deepseekv4pro":   "deepseek/deepseek-v4-pro",
	"deepseekv4":      "deepseek/deepseek-v4-pro",  // 模糊短名 → 默认 Pro
	"deepseekpro":     "deepseek/deepseek-v4-pro",
	"deepseekflash":   "deepseek/deepseek-v4-flash",
	// Google
	"gemini31flashlite": "google/gemini-3.1-flash-lite",
	"gemini35flash":     "google/gemini-3.5-flash",
	"gemini35flashlite": "google/gemini-3.5-flash-lite",
	"gemini36flash":     "google/gemini-3.6-flash",
	// InclusionAI
	"ling30flash":     "inclusionai/ling-3.0-flash-free",
	"ling30flashfree": "inclusionai/ling-3.0-flash-free",
	"ling":            "inclusionai/ling-3.0-flash-free",
	// Meta (Muse Spark)
	"musespark11":           "meta/muse-spark-1.1",
	"musespark12":           "meta/muse-spark-1.2",
	"musespark12contributor": "meta/muse-spark-1.2-contributor",
	// MiniMax
	"minimaxm25": "MiniMaxAI/MiniMax-M2.5",
	"minimaxm27": "MiniMaxAI/MiniMax-M2.7",
	"minimaxm3":  "MiniMaxAI/MiniMax-M3",
	"minimax3":   "MiniMaxAI/MiniMax-M3",
	// Moonshot AI
	"kimik25":              "moonshotai/Kimi-K2.5",
	"kimik26":              "moonshotai/Kimi-K2.6",
	"kimik27code":          "moonshotai/Kimi-K2.7-Code",
	"kimik27codehighspeed": "moonshotai/Kimi-K2.7-Code-Highspeed",
	"kimik3":               "moonshotai/Kimi-K3",
	// NVIDIA
	"nemotron3ultra":           "nvidia/nemotron-3-ultra-550b-a55b",
	"nemotron3ultra550ba55b": "nvidia/nemotron-3-ultra-550b-a55b",
	"nemotron":                "nvidia/nemotron-3-ultra-550b-a55b",
	// OpenAI
	"gpt53codex": "gpt-5.3-codex",
	"gpt54":      "gpt-5.4",
	"gpt54mini":  "gpt-5.4-mini",
	"gpt55":      "gpt-5.5",
	"gpt56luna":  "gpt-5.6-luna",
	"gpt56sol":   "gpt-5.6-sol",
	"gpt56terra": "gpt-5.6-terra",
	// Poolside
	"lagunas21":   "poolside/laguna-s-2.1-free",
	"lagunas21free": "poolside/laguna-s-2.1-free",
	"laguna":      "poolside/laguna-s-2.1-free",
	// Sakana AI
	"fuguultra": "sakana/fugu-ultra",
	"fugu":      "sakana/fugu-ultra",
	// StepFun
	"step35flash": "stepfun/Step-3.5-Flash",
	"step37flash": "stepfun/Step-3.7-Flash",
	// Tencent
	"hy3":    "tencent/hy3-paid",
	"hy3paid": "tencent/hy3-paid",
	// Thinking Machines
	"inkling":      "thinkingmachines/inkling",
	"inklingsmall": "thinkingmachines/inkling-small",
	// xAI
	"grok45": "xai/grok-4.5",
	"grok":   "xai/grok-4.5",
	// Xiaomi
	"mimov25":    "xiaomi/mimo-v2.5",
	"mimov25pro": "xiaomi/mimo-v2.5-pro",
	"mimo":       "xiaomi/mimo-v2.5",
	// Z AI
	"glm5":     "zai-org/GLM-5",
	"glm51":    "zai-org/GLM-5.1",
	"glm52":    "zai-org/GLM-5.2",
	"glm52fast": "zai-org/GLM-5.2-Fast",
}