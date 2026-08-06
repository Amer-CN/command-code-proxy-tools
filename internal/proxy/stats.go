package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"started":   snap.Started,
		"models":    snap.Models,
		"total":     map[string]int64{"input": totalIn, "output": totalOut, "total": totalIn + totalOut},
		"today":     map[string]int64{"input": todayIn, "output": todayOut, "total": todayIn + todayOut},
		"statsFile": filepath.Base(p.Stats.file),
	})
}
