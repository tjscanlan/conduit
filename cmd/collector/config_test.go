package main

import (
	"testing"

	"github.com/your-org/agent-observability/internal/storage"
)

func TestRetentionConfig(t *testing.T) {
	for v, want := range map[string]int{"": storage.DefaultRetentionDays, "30": 30, "abc": -1, "0": 0} {
		t.Setenv("SPAN_RETENTION_DAYS", v)
		if got := loadConfig().RetentionDays; got != want {
			t.Errorf("SPAN_RETENTION_DAYS=%q -> %d, want %d", v, got, want)
		}
	}
}
