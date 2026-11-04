package storage

import (
	"strings"
	"testing"
	"time"
)

func TestPromLineEscapesLabelValues(t *testing.T) {
	ts := time.UnixMilli(1700000000000)
	for _, tc := range []struct {
		name, value, want string
	}{
		{"plain", "llm.call", `conduit_span_duration_ms{agent="bot",span="llm.call"} 12 1700000000000` + "\n"},
		{"quote", `say "hi"`, `conduit_span_duration_ms{agent="bot",span="say \"hi\""} 12 1700000000000` + "\n"},
		{"backslash", `C:\tmp`, `conduit_span_duration_ms{agent="bot",span="C:\\tmp"} 12 1700000000000` + "\n"},
		// Injection attempts: must stay inside the label value, on one line.
		{"newline injection", "x\"} 1 0\nconduit_span_duration_ms{agent=\"victim\"", `conduit_span_duration_ms{agent="bot",span="x\"} 1 0\nconduit_span_duration_ms{agent=\"victim\""} 12 1700000000000` + "\n"},
		{"backslash-quote breakout", `a\"} 9 0`, `conduit_span_duration_ms{agent="bot",span="a\\\"} 9 0"} 12 1700000000000` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := promLine(VMMetric{Name: "conduit_span_duration_ms", Labels: map[string]string{"span": tc.value, "agent": "bot"}, Value: 12, Timestamp: ts})
			if got != tc.want {
				t.Errorf("promLine:\n got %q\nwant %q", got, tc.want)
			}
			if strings.Count(got, "\n") != 1 {
				t.Errorf("sample spans %d lines; a label value injected a new sample", strings.Count(got, "\n"))
			}
		})
	}
}
