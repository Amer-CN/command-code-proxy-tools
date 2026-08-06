package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUsageStatsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stats.json")

	s := NewUsageStats(file)
	s.Record("deepseek/deepseek-v4-flash", 1000, 500)
	s.Record("deepseek/deepseek-v4-flash", 2000, 800)

	// Simulate restart: new instance loading the same file.
	s2 := NewUsageStats(file)
	if s2.Models["deepseek/deepseek-v4-flash"] == nil {
		t.Fatal("stats lost after reload")
	}
	ms := s2.Models["deepseek/deepseek-v4-flash"]
	if ms.InputTokens != 3000 || ms.OutputTokens != 1300 || ms.Runs != 2 {
		t.Fatalf("unexpected totals: %+v", ms)
	}
	todayIn, todayOut := s2.Today()
	if todayIn != 3000 || todayOut != 1300 {
		t.Fatalf("unexpected today: in=%d out=%d", todayIn, todayOut)
	}
	_ = os.Remove(file)
}
