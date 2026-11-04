package storage

import (
	"strings"
	"testing"
)

func TestTTLDaysParsesClickHouseEngineFull(t *testing.T) {
	// Verbatim engine_full strings from ClickHouse 24.4 (before and after MODIFY TTL).
	for s, want := range map[string]int{
		"MergeTree ORDER BY start_time TTL toDateTime(start_time) + toIntervalDay(90) SETTINGS index_granularity = 8192":                                                             90,
		"MergeTree PARTITION BY toYYYYMMDD(start_time) ORDER BY (agent_name, trace_id, start_time) TTL toDateTime(start_time) + toIntervalDay(30) SETTINGS index_granularity = 8192": 30,
		// logs table (0003), verbatim after MODIFY TTL on ClickHouse 24.4
		"MergeTree PARTITION BY toYYYYMMDD(timestamp) ORDER BY (agent_name, timestamp) TTL toDateTime(timestamp) + toIntervalDay(30) SETTINGS index_granularity = 8192": 30,
	} {
		if got, ok := ttlDays(s); !ok || got != want {
			t.Errorf("ttlDays(%q) = %d, %v; want %d", s, got, ok, want)
		}
	}
	if _, ok := ttlDays("MergeTree ORDER BY x SETTINGS index_granularity = 8192"); ok {
		t.Error("table without TTL should not parse")
	}
}

func TestNewClickHouseStoreRejectsBadRetention(t *testing.T) {
	for _, d := range []int{-1, MaxRetentionDays + 1} {
		if _, err := NewClickHouseStore("clickhouse://localhost:1/db", StoreOptions{RetentionDays: d}); err == nil || !strings.Contains(err.Error(), "retention") {
			t.Errorf("retention %d: err = %v, want a range error before connecting", d, err)
		}
	}
}

func TestClickHouseOptionsKeepTLS(t *testing.T) {
	// Regression: TLS used to be forced off, downgrading secure DSNs to plaintext.
	o, err := clickhouseOptions("clickhouse://u:p@ch.example:9440/db?secure=true")
	if err != nil {
		t.Fatal(err)
	}
	if o.TLS == nil {
		t.Fatal("secure=true DSN lost its TLS config")
	}
	o, _ = clickhouseOptions("clickhouse://u:p@localhost:9000/db")
	if o.TLS != nil {
		t.Error("plain DSN should not enable TLS")
	}
}
