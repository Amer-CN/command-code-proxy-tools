package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// UsageStats is a thread-safe local counter of tokens that flowed through the
// proxy. It accumulates real usage reported by CommandCode in stream events
// and persists to disk so counts survive proxy restarts.
type UsageStats struct {
	mu      sync.Mutex
	file    string                `json:"-"`
	Models  map[string]*ModelStat `json:"models"`
	Started int64                 `json:"started"` // unix seconds
}

// ModelStat tracks token counts for one model.
type ModelStat struct {
	Runs         int64 `json:"runs"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// Per-day breakdown, keyed by YYYY-MM-DD (local time).
	Days map[string]*DayStat `json:"days,omitempty"`
}

// DayStat tracks one calendar day's usage.
type DayStat struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
}

func NewUsageStats(file string) *UsageStats {
	s := &UsageStats{Models: map[string]*ModelStat{}, file: file}
	s.load()
	return s
}

// dayKey returns today's key in local time, e.g. "2026-08-06".
func dayKey() string {
	return time.Now().Format("2006-01-02")
}

// Record adds one completed run's usage for a model.
func (s *UsageStats) Record(model string, input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms := s.Models[model]
	if ms == nil {
		ms = &ModelStat{}
		s.Models[model] = ms
	}
	ms.Runs++
	ms.InputTokens += input
	ms.OutputTokens += output
	if ms.Days == nil {
		ms.Days = map[string]*DayStat{}
	}
	dk := dayKey()
	ds := ms.Days[dk]
	if ds == nil {
		ds = &DayStat{}
		ms.Days[dk] = ds
	}
	ds.InputTokens += input
	ds.OutputTokens += output
	s.save()
}

// Today returns aggregate usage for today across all models.
func (s *UsageStats) Today() (in, out int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dk := dayKey()
	for _, ms := range s.Models {
		if ds := ms.Days[dk]; ds != nil {
			in += ds.InputTokens
			out += ds.OutputTokens
		}
	}
	return
}

// Snapshot returns a copy for JSON output.
func (s *UsageStats) Snapshot() *UsageStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &UsageStats{Started: s.Started, Models: map[string]*ModelStat{}}
	for k, v := range s.Models {
		c := *v
		if v.Days != nil {
			c.Days = map[string]*DayStat{}
			for dk, dv := range v.Days {
				d := *dv
				c.Days[dk] = &d
			}
		}
		out.Models[k] = &c
	}
	return out
}

// load reads persisted stats from disk, if present.
func (s *UsageStats) load() {
	if s.file == "" {
		return
	}
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, s)
	if s.Models == nil {
		s.Models = map[string]*ModelStat{}
	}
}

// save writes stats to disk atomically.
func (s *UsageStats) save() {
	if s.file == "" {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.file)
}

// HandleStats returns accumulated local usage as JSON.
func (p *Proxy) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		p.writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error")
		return
	}
	snap := p.Stats.Snapshot()
	todayIn, todayOut := p.Stats.Today()
	var totalIn, totalOut int64
	for _, ms := range snap.Models {
		totalIn += ms.InputTokens
		totalOut += ms.OutputTokens
	}

	// 金额估算（官方定价 per 1M tokens，USD；无价格的模型按 0 计）
	cost := estimateCost(snap.Models)

	// 自定义校准值（用户从官网读取后填入，保存到 calibration.txt）
	calib := p.calibration()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"started":   snap.Started,
		"models":    snap.Models,
		"total":     map[string]int64{"input": totalIn, "output": totalOut, "total": totalIn + totalOut},
		"today":     map[string]int64{"input": todayIn, "output": todayOut, "total": todayIn + todayOut},
		"statsFile": filepath.Base(p.Stats.file),
		"cost":      cost, // 估算金额（按官方单价 × 本地 token）
		"calibration": map[string]any{
			"official": calib, // 用户填的官网金额（0 = 未填）
		},
	})
}

// pricing 按模型 ID（短名或全名）返回每百万 token 的输入/输出价格（USD）。
// 来源：https://commandcode.ai/docs/resources/pricing-limits（2026-08 官方定价）
var pricing = map[string][2]float64{ // [输入, 输出] per 1M tokens
	"deepseek-v4-flash":   {0.14, 0.28},
	"deepseek/deepseek-v4-flash": {0.14, 0.28},
	"deepseek-v4-pro":     {0.435, 0.87},
	"deepseek/deepseek-v4-pro":   {0.435, 0.87},
	"kimi-k2.6":           {0.95, 4.00},
	"moonshotai/kimi-k2.6": {0.95, 4.00},
	"kimi-k2.5":           {0.60, 3.00},
	"moonshotai/kimi-k2.5": {0.60, 3.00},
	"glm-5.1":             {1.40, 4.40},
	"zai-org/glm-5.1":     {1.40, 4.40},
	"glm-5":               {1.00, 3.20},
	"zai-org/glm-5":       {1.00, 3.20},
	"minimax-m3":          {0.30, 1.20},
	"minimaxai/minimax-m3": {0.30, 1.20},
	"minimax-m2.7":        {0.30, 1.20},
	"minimaxai/minimax-m2.7": {0.30, 1.20},
	"minimax-m2.5":        {0.30, 1.20},
	"minimaxai/minimax-m2.5": {0.30, 1.20},
	"qwen-3.7-max":        {2.50, 7.50},
	"qwen/qwen3.7-max":    {2.50, 7.50},
	"qwen-3.7-max-free":   {0.0, 0.0},
	"qwen/qwen3.7-max-free": {0.0, 0.0},
	"qwen-3.6-max":        {1.30, 7.80},
	"qwen/qwen3.6-max-preview": {1.30, 7.80},
	"qwen-3.6-plus":       {0.50, 3.00},
	"qwen/qwen3.6-plus":   {0.50, 3.00},
	"step-3.7-flash":      {0.20, 1.15},
	"stepfun/step-3.7-flash": {0.20, 1.15},
	"step-3.5-flash":      {0.10, 0.30},
	"stepfun/step-3.5-flash": {0.10, 0.30},
	"mimo-v2.5-pro":       {0.435, 0.87},
	"xiaomi/mimo-v2.5-pro": {0.435, 0.87},
	"mimo-v2.5":           {0.14, 0.28},
	"xiaomi/mimo-v2.5":    {0.14, 0.28},
	"gemini-3.1-flash-lite": {0.0, 0.0},
	"google/gemini-3.1-flash-lite": {0.0, 0.0},
}

// estimateCost 按模型价格表估算总金额（USD）。
func estimateCost(models map[string]*ModelStat) float64 {
	var total float64
	for name, ms := range models {
		pr := pricing[name]
		if pr[0] == 0 && pr[1] == 0 {
			pr = pricing[shortName(name)]
		}
		total += float64(ms.InputTokens)/1e6*pr[0] + float64(ms.OutputTokens)/1e6*pr[1]
	}
	return total
}

// shortName 从全名（如 "deepseek/deepseek-v4-flash"）取短名。
func shortName(full string) string {
	if i := strings.LastIndex(full, "/"); i >= 0 {
		return full[i+1:]
	}
	return full
}

// calibration 读取用户填写的官网校准金额（USD），文件与 stats.json 同目录。
func (p *Proxy) calibration() float64 {
	dir := filepath.Dir(p.Stats.file)
	if dir == "." {
		dir = "."
	}
	b, err := os.ReadFile(filepath.Join(dir, "calibration.txt"))
	if err != nil {
		return 0
	}
	var v float64
	_, _ = fmt.Sscanf(strings.TrimSpace(string(b)), "%f", &v)
	return v
}
