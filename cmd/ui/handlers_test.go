package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/your-org/agent-observability/internal/storage"
)

type fakeSpans struct {
	rows       []storage.SpanRow
	lastFilter storage.TraceFilter
}

// QueryTraces mirrors the SQL: whole traces that include the agent, rolled up
// from all their spans, newest first.
func (f *fakeSpans) QueryTraces(_ context.Context, tf storage.TraceFilter) ([]storage.TraceSummary, error) {
	f.lastFilter = tf
	byTrace := map[string]*storage.TraceSummary{}
	hasAgent := map[string]bool{}
	var order []string
	for _, r := range f.rows {
		t, ok := byTrace[r.TraceID]
		if !ok {
			t = &storage.TraceSummary{TraceID: r.TraceID, Started: r.StartTime, Ended: r.EndTime, RootName: r.SpanName}
			byTrace[r.TraceID] = t
			order = append(order, r.TraceID)
		}
		t.SpanCount++
		if r.Status == "error" {
			t.ErrorCount++
		}
		if r.ParentID == "" {
			t.RootName = r.SpanName
		}
		if r.AgentName == tf.Agent {
			hasAgent[r.TraceID] = true
		}
		t.Agents = append(t.Agents, r.AgentName)
	}
	var out []storage.TraceSummary
	for i := len(order) - 1; i >= 0; i-- {
		t := byTrace[order[i]]
		if (tf.Agent != "" && !hasAgent[t.TraceID]) || (tf.ErrorsOnly && t.ErrorCount == 0) {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
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
	tmpls := parseTemplates()
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

func listRouter(t *testing.T, rows []storage.SpanRow) (http.Handler, *fakeSpans) {
	t.Helper()
	tmpls := parseTemplates()
	a, err := newUIAuth(&Config{AuthMode: authModeNone}, tmpls, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeSpans{rows: rows}
	return newRouter(a, &handlers{ch: fs, tmpls: tmpls, logger: testLogger()}), fs
}

func TestTraceListOneRowPerTrace(t *testing.T) {
	router, fs := listRouter(t, sampleRows())
	body := do(router, httptest.NewRequest(http.MethodGet, "/traces", nil)).Body.String()
	// Two traces (aaaa has two spans): two rows, linked to the detail page by root name.
	if n := strings.Count(body, `class="span-row`); n != 2 {
		t.Errorf("rendered %d rows, want one per trace (2)", n)
	}
	for _, want := range []string{`href="/traces/aaaa">agent.run</a>`, `href="/traces/bbbb">other.trace</a>`, "badge-error"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	if strings.Contains(body, ">llm.call<") {
		t.Error("child spans should not get their own rows")
	}
	if fs.lastFilter.Window != 24*time.Hour || fs.lastFilter.Limit != traceListLimit {
		t.Errorf("default filter = %+v", fs.lastFilter)
	}
	if !strings.Contains(body, `hx-get="/traces?window=24h"`) {
		t.Error("list should poll with its filter")
	}
}

func TestTraceListFilters(t *testing.T) {
	router, fs := listRouter(t, sampleRows())
	body := do(router, httptest.NewRequest(http.MethodGet, "/traces?window=1h&errors=1&agent=bot", nil)).Body.String()
	if fs.lastFilter.Window != time.Hour || !fs.lastFilter.ErrorsOnly || fs.lastFilter.Agent != "bot" {
		t.Errorf("filter = %+v", fs.lastFilter)
	}
	if !strings.Contains(body, "agent.run") || strings.Contains(body, "other.trace") {
		t.Error("errors-only should keep aaaa and drop bbbb")
	}
	// The form reflects the active filter, and polling keeps it.
	for _, want := range []string{`value="bot"`, "checked", `value="1h" selected`, `hx-get="/traces?agent=bot&amp;errors=1&amp;window=1h"`, `href="/traces">Clear`} {
		if !strings.Contains(body, want) {
			t.Errorf("filtered page missing %q", want)
		}
	}

	body = do(router, httptest.NewRequest(http.MethodGet, "/traces?agent=nobody", nil)).Body.String()
	if !strings.Contains(body, "No traces match these filters") {
		t.Error("empty filtered result should say so")
	}
}

func TestParseTraceListFilterRejectsBadInput(t *testing.T) {
	for q, want := range map[string]traceListFilter{
		"window=30d":                            {Window: "24h"},
		"window=1h%27--":                        {Window: "24h"},
		"errors=true":                           {Window: "24h"},
		"agent=" + strings.Repeat("a", 257):     {Window: "24h"},
		"agent=%ff%fe":                          {Window: "24h"},
		"window=7d&agent=research-bot&errors=1": {Window: "7d", Agent: "research-bot", ErrorsOnly: true},
	} {
		v, _ := url.ParseQuery(q)
		if got := parseTraceListFilter(v); got != want {
			t.Errorf("%s: got %+v, want %+v", q, got, want)
		}
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
