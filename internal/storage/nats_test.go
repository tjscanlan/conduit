package storage

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestSpanStreamIsBoundedWorkQueue(t *testing.T) {
	cfg := spanStreamConfig()
	if cfg.Retention != nats.WorkQueuePolicy {
		t.Errorf("retention = %v, want work queue (acked spans deleted)", cfg.Retention)
	}
	if cfg.MaxAge <= 0 {
		t.Error("MaxAge must bound the stream")
	}
	if len(cfg.Subjects) != 1 || cfg.Subjects[0] != SpanSubject {
		t.Errorf("subjects = %v", cfg.Subjects)
	}
}
