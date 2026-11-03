package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOTLPPayloadStatus(t *testing.T) {
	span := func(status string) map[string]any {
		p := buildOTLPPayload(TempoSpan{TraceID: "t", SpanID: "s", Name: "n", Status: status})
		return p["resourceSpans"].([]map[string]any)[0]["scopeSpans"].([]map[string]any)[0]["spans"].([]map[string]any)[0]
	}
	if _, ok := span("unset")["status"]; ok {
		t.Error("unset status should be omitted (OTLP default)")
	}
	for status, code := range map[string]int{"ok": 1, "error": 2} {
		got, _ := span(status)["status"].(map[string]any)
		if got["code"] != code {
			t.Errorf("%s: status = %v, want code %d", status, got, code)
		}
	}
}

func TestWriteSpanPostsToOTLPHTTPPath(t *testing.T) {
	var gotPath, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotType = r.URL.Path, r.Header.Get("Content-Type")
	}))
	defer srv.Close()

	c := NewTempoClient(srv.URL + "/")
	if err := c.WriteSpan(context.Background(), TempoSpan{TraceID: "t", SpanID: "s", Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/traces" || gotType != "application/json" {
		t.Errorf("POST %s (%s), want /v1/traces (application/json), Tempo's OTLP/HTTP receiver", gotPath, gotType)
	}
}
