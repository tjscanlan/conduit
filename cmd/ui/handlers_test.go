package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/your-org/agent-observability/internal/storage"
	conduitui "github.com/your-org/agent-observability/ui"
)

type fakeSpans struct{ rows []storage.SpanRow }

func (f *fakeSpans) QueryRecentSpans(context.Context, int) ([]storage.SpanRow, error) {
	return f.rows, nil
}

func (f *fakeSpans) QueryTrace(_ context.Context, id string, limit int) ([]storage.SpanRow, error) {
	var out []storage.SpanRow
	for _, r := range f.rows {
		if r.TraceID == id && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func newDataRouter(t *testing.T, rows []storage.SpanRow) http.Handler {
	t.Helper()
	tmpls := template.Must(template.ParseFS(conduitui.Assets, "templates/*.html"))
	a, err := newUIAuth(&Config{AuthMode: authModeNone}, tmpls, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	return newRouter(a, &handlers{ch: &fakeSpans{rows: rows}, tmpls: tmpls, logger: testLogger()})
}

func sampleRows() []storage.SpanRow {
	now := time.Now()
	return []storage.SpanRow{
		{TraceID: "aaaa", SpanID: "s1", SpanName: "agent.run", AgentName: "bot", Status: "ok", StartTime: now},
		{TraceID: "aaaa", SpanID: "s2", ParentID: "s1", SpanName: "llm.call", AgentName: "bot", Status: "error", StartTime: now},
		{TraceID: "bbbb", SpanID: "s3", SpanName: "other.trace", AgentName: "bot", Status: "ok", StartTime: now},
	}
}

func TestTraceDetailShowsOnlyThatTrace(t *testing.T) {
	rec := do(newDataRouter(t, sampleRows()), httptest.NewRequest(http.MethodGet, "/traces/aaaa", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"agent.run", "llm.call", "badge-error", "aaaa", "depth-1", `class="waterfall"`, "<details>"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
	if strings.Contains(body, "other.trace") || strings.Contains(body, "No spans yet") {
		t.Error("detail page shows other traces or the empty state")
	}
	// Polling /traces would replace the detail view with the global list.
	if strings.Contains(body, `hx-get="/traces"`) {
		t.Error("detail page must not poll the global trace list")
	}
}

func TestTraceDetailUnknownTraceIs404(t *testing.T) {
	rec := do(newDataRouter(t, sampleRows()), httptest.NewRequest(http.MethodGet, "/traces/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestTraceListPolls(t *testing.T) {
	body := do(newDataRouter(t, sampleRows()), httptest.NewRequest(http.MethodGet, "/traces", nil)).Body.String()
	if !strings.Contains(body, "other.trace") || !strings.Contains(body, `hx-get="/traces"`) {
		t.Error("list page should show all spans and poll for updates")
	}
}

func TestTraceDetailTruncatesHugeTraces(t *testing.T) {
	var rows []storage.SpanRow
	now := time.Now()
	for i := 0; i < maxTraceSpans+50; i++ {
		rows = append(rows, storage.SpanRow{TraceID: "big", SpanID: fmt.Sprintf("s%05d", i), SpanName: "step",
			Status: "ok", StartTime: now.Add(time.Duration(i) * time.Millisecond), EndTime: now.Add(time.Duration(i+1) * time.Millisecond)})
	}
	body := do(newDataRouter(t, rows), httptest.NewRequest(http.MethodGet, "/traces/big", nil)).Body.String()
	if n := strings.Count(body, `class="span-node`); n != maxTraceSpans {
		t.Errorf("rendered %d spans, want %d", n, maxTraceSpans)
	}
	if !strings.Contains(body, "more than 2000 spans") {
		t.Error("truncation notice missing")
	}

	small := do(newDataRouter(t, sampleRows()), httptest.NewRequest(http.MethodGet, "/traces/aaaa", nil)).Body.String()
	if strings.Contains(small, "more than") {
		t.Error("notice shown for an untruncated trace")
	}
	if !strings.Contains(small, `id="span-s2"`) || !strings.Contains(small, `href="#span-s2"`) {
		t.Error("span deep-link anchors missing")
	}
}
