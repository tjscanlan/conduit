package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/schema"
	"github.com/tjscanlan/conduit/internal/storage"
)

func TestProcessLogDefaults(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	req, err := ProcessLog(LogInput{Body: "hello", AgentName: "bot"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if req.Severity != schema.SeverityUnspecified || !req.Timestamp.Equal(now) || req.Attributes == nil {
		t.Errorf("defaults not applied: %+v", req)
	}
	ts := now.Add(-time.Minute)
	req, _ = ProcessLog(LogInput{Body: "x", Severity: "error", TimestampNano: ts.UnixNano()}, now)
	if !req.Timestamp.Equal(ts) || req.Severity != "error" {
		t.Errorf("explicit values lost: %+v", req)
	}
}

type fakeLogStore struct {
	err  error
	rows []storage.LogRow
}

func (f *fakeLogStore) InsertLog(_ context.Context, row storage.LogRow) error {
	f.rows = append(f.rows, row)
	return f.err
}

func TestCollectorHandleLogDispositions(t *testing.T) {
	good, _ := json.Marshal(LogInput{Body: "ok", Severity: "info", AgentName: "bot"})
	bad, _ := json.Marshal(LogInput{Body: "", Severity: "info"})
	for _, tt := range []struct {
		name     string
		data     []byte
		storeErr error
		want     disposition
		stored   int
	}{
		{"stored", good, nil, ack, 1},
		{"store failure retries", good, errors.New("clickhouse down"), retry, 1},
		{"undecodable terminated", []byte("{"), nil, term, 0},
		{"invalid acked, not stored", bad, nil, ack, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeLogStore{err: tt.storeErr}
			c := &Collector{logs: store, logger: discardLogger()}
			if got := c.handleLog(context.Background(), tt.data); got != tt.want {
				t.Errorf("disposition = %v, want %v", got, tt.want)
			}
			if len(store.rows) != tt.stored {
				t.Errorf("stored %d, want %d", len(store.rows), tt.stored)
			}
			if tt.stored == 1 && (store.rows[0].AgentName != "bot" || store.rows[0].Body != "ok") {
				t.Errorf("row = %+v", store.rows[0])
			}
		})
	}
}
