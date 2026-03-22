package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TempoClient writes trace data to Grafana Tempo via OTLP/HTTP.
type TempoClient struct {
	endpoint   string
	httpClient *http.Client
}

func NewTempoClient(endpoint string) *TempoClient {
	return &TempoClient{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type TempoSpan struct {
	TraceID   string
	SpanID    string
	ParentID  string
	Name      string
	StartNano int64
	EndNano   int64
	Status    string
}

// WriteSpan sends a single span to Tempo in OTLP JSON format.
func (c *TempoClient) WriteSpan(ctx context.Context, s TempoSpan) error {
	payload := buildOTLPPayload(s)
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/otlp/v1/traces", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("tempo: status %d", resp.StatusCode)
	}
	return nil
}

func buildOTLPPayload(s TempoSpan) map[string]any {
	span := map[string]any{
		"traceId":           s.TraceID,
		"spanId":            s.SpanID,
		"name":              s.Name,
		"startTimeUnixNano": fmt.Sprintf("%d", s.StartNano),
		"endTimeUnixNano":   fmt.Sprintf("%d", s.EndNano),
	}
	if s.ParentID != "" {
		span["parentSpanId"] = s.ParentID
	}

	return map[string]any{
		"resourceSpans": []map[string]any{{
			"scopeSpans": []map[string]any{{
				"spans": []map[string]any{span},
			}},
		}},
	}
}
