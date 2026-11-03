package storage

import "testing"

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
