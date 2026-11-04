package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

type fakeSpans struct {
	rows          []storage.SpanRow
	lastFilter    storage.TraceFilter
	agentsErr     error
	agentCalls    int
	readyErr      error
	logs          []storage.LogRow
	lastLogFilter storage.LogFilter

	onboardingErr   error
	onboardingCalls int
}

func (f *fakeSpans) Ready(context.Context) error { return f.readyErr }

// QueryOnboarding mirrors the SQL over f.rows: recent counts, and the newest
// span from a non-demo agent.
func (f *fakeSpans) QueryOnboarding(_ context.Context, demo string, recent, _ time.Duration) (storage.Onboarding, error) {
	f.onboardingCalls++
	if f.onboardingErr != nil {
		return storage.Onboarding{}, f.onboardingErr
	}
	var o storage.Onboarding
	for _, r := range f.rows {
		if time.Since(r.StartTime) <= recent {
			o.RecentSpans++
			if r.AgentName == demo {
				o.RecentDemo++
			}
		}
		if r.AgentName != "" && r.AgentName != demo && r.StartTime.After(o.LatestAt) {
			o.LatestTraceID, o.LatestAgent, o.LatestAt = r.TraceID, r.AgentName, r.StartTime
		}
	}
	return o, nil
}

// QueryLogs mirrors the SQL filters on f.logs, newest first as stored.
func (f *fakeSpans) QueryLogs(_ context.Context, lf storage.LogFilter) ([]storage.LogRow, error) {
	f.lastLogFilter = lf
	var out []storage.LogRow
	for _, l := range f.logs {
		if (lf.Agent != "" && l.AgentName != lf.Agent) || (lf.TraceID != "" && l.TraceID != lf.TraceID) ||
			(lf.Search != "" && !strings.Contains(strings.ToLower(l.Body), strings.ToLower(lf.Search))) {
			continue
		}
		if len(lf.Severities) > 0 && !slices.Contains(lf.Severities, l.Severity) {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (f *fakeSpans) QueryAgents(_ context.Context, _ time.Duration, limit int) ([]string, error) {
	f.agentCalls++
	if f.agentsErr != nil {
		return nil, f.agentsErr
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range f.rows {
		if r.AgentName != "" && !seen[r.AgentName] && len(out) < limit {
			seen[r.AgentName] = true
			out = append(out, r.AgentName)
		}
	}
	sort.Strings(out)
	return out, nil
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
		if !tf.BeforeStarted.IsZero() && !(t.Started.Before(tf.BeforeStarted) ||
			(t.Started.Equal(tf.BeforeStarted) && t.TraceID < tf.BeforeTraceID)) {
			continue
		}
		out = append(out, *t)
		if len(out) == tf.Limit {
			break
		}
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
	if fs.lastFilter.Window != 24*time.Hour || fs.lastFilter.Limit != traceListLimit+1 {
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

func manyTraces(n int) []storage.SpanRow {
	var rows []storage.SpanRow
	t0 := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ { // ascending start: trace i starts at t0 + i ms
		rows = append(rows, storage.SpanRow{TraceID: fmt.Sprintf("t%04d", i), SpanID: "s", SpanName: fmt.Sprintf("root-%d", i),
			Status: "ok", StartTime: t0.Add(time.Duration(i) * time.Millisecond), EndTime: t0.Add(time.Duration(i+1) * time.Millisecond)})
	}
	return rows
}

func TestTraceListPaging(t *testing.T) {
	router, fs := listRouter(t, manyTraces(traceListLimit+30))

	first := do(router, httptest.NewRequest(http.MethodGet, "/traces", nil)).Body.String()
	if n := strings.Count(first, `class="span-row`); n != traceListLimit {
		t.Fatalf("first page has %d rows, want %d", n, traceListLimit)
	}
	if fs.lastFilter.Limit != traceListLimit+1 {
		t.Errorf("should fetch one extra row to detect more pages, got limit %d", fs.lastFilter.Limit)
	}
	if strings.Contains(first, "Newest</a>") || !strings.Contains(first, "Older →") {
		t.Error("first page should link Older only")
	}
	m := regexp.MustCompile(`href="/traces\?([^"]+)">Older`).FindStringSubmatch(first)
	if m == nil {
		t.Fatal("no Older link")
	}
	if strings.Contains(m[1], "%3d") || strings.Contains(m[1], "%26") {
		t.Fatalf("Older link query was double-encoded: %s", m[1])
	}
	olderURL := "/traces?" + strings.ReplaceAll(m[1], "&amp;", "&")

	second := do(router, httptest.NewRequest(http.MethodGet, olderURL, nil)).Body.String()
	if n := strings.Count(second, `class="span-row`); n != 30 {
		t.Errorf("second page has %d rows, want the remaining 30", n)
	}
	// Newest-first: page 1 ends at root-30, page 2 starts at root-29 with no overlap or gap.
	if !strings.Contains(first, ">root-30<") || strings.Contains(first, ">root-29<") ||
		!strings.Contains(second, ">root-29<") || strings.Contains(second, ">root-30<") || !strings.Contains(second, ">root-0<") {
		t.Error("pages overlap or skip a trace at the boundary")
	}
	if strings.Contains(second, "Older →") || !strings.Contains(second, "Newest</a>") {
		t.Error("last page should link back to Newest only")
	}
	if strings.Contains(second, `hx-get=`) {
		t.Error("an older page must not auto-refresh")
	}
}

func TestTraceCursorParsing(t *testing.T) {
	ts := time.Date(2026, 11, 3, 12, 0, 0, 123456789, time.UTC)
	c := traceCursor(storage.TraceSummary{TraceID: "abc123", Started: ts})
	got, id, ok := parseTraceCursor(c)
	if !ok || !got.Equal(ts) || id != "abc123" {
		t.Fatalf("round trip %q -> %v %q %v", c, got, id, ok)
	}
	for _, bad := range []string{"", "123", ".abc", "abc.def", "-5.abc", "0.abc", "123.a b", "123.x'--", "123." + strings.Repeat("a", 65)} {
		if _, _, ok := parseTraceCursor(bad); ok {
			t.Errorf("cursor %q accepted", bad)
		}
	}
	if f := parseTraceListFilter(url.Values{"before": {"123.x'--"}}); f.Before != "" {
		t.Error("invalid cursor kept in filter")
	}
}

func TestTraceListAgentSuggestions(t *testing.T) {
	rows := sampleRows()
	rows[2].AgentName = "research-bot"
	router, fs := listRouter(t, rows)
	body := do(router, httptest.NewRequest(http.MethodGet, "/traces", nil)).Body.String()
	for _, want := range []string{`list="agent-options"`, `<option value="bot">`, `<option value="research-bot">`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}

	// htmx polling only swaps the rows, so it must not pay for the agent query.
	fs.agentCalls = 0
	req := httptest.NewRequest(http.MethodGet, "/traces", nil)
	req.Header.Set("HX-Request", "true")
	do(router, req)
	if fs.agentCalls != 0 {
		t.Error("htmx partial queried agents")
	}

	// A failing suggestions query must not break the page.
	fs.agentsErr = errors.New("clickhouse down")
	rec := do(router, httptest.NewRequest(http.MethodGet, "/traces", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "agent.run") {
		t.Errorf("page failed when agent suggestions failed: %d", rec.Code)
	}
}

func TestReadyzUnauthenticated(t *testing.T) {
	// Login mode: /readyz must not require a session (probes have none).
	router, _ := newTestRouter(t, authModeLogin)
	if rec := do(router, httptest.NewRequest(http.MethodGet, "/readyz", nil)); rec.Code != http.StatusOK {
		t.Fatalf("/readyz = %d, want 200 without auth", rec.Code)
	}

	r, fs := listRouter(t, nil)
	fs.readyErr = errors.New("clickhouse down")
	if rec := do(r, httptest.NewRequest(http.MethodGet, "/readyz", nil)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz with ClickHouse down = %d, want 503", rec.Code)
	}
}
