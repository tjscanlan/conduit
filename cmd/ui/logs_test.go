package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

func logsRouter(t *testing.T, logs []storage.LogRow) (http.Handler, *fakeSpans) {
	t.Helper()
	r, fs := listRouter(t, nil)
	fs.logs = logs
	return r, fs
}

func sampleLogs() []storage.LogRow {
	now := time.Now()
	return []storage.LogRow{
		{Timestamp: now, AgentName: "bot", Severity: "error", Body: "tool <script>alert(1)</script> failed", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7"},
		{Timestamp: now.Add(-time.Second), AgentName: "bot", Severity: "warn", Body: "retrying tool call", TraceID: "abc"},
		{Timestamp: now.Add(-2 * time.Second), AgentName: "other", Severity: "info", Body: "agent started"},
	}
}

func TestLogsPage(t *testing.T) {
	router, fs := logsRouter(t, sampleLogs())
	body := do(router, httptest.NewRequest(http.MethodGet, "/logs", nil)).Body.String()
	if fs.lastLogFilter.Window != 24*time.Hour || fs.lastLogFilter.Limit != logListLimit || len(fs.lastLogFilter.Severities) != 0 {
		t.Errorf("default filter = %+v", fs.lastLogFilter)
	}
	for _, want := range []string{
		"retrying tool call", "agent started", "sev-badge-error", "sev-badge-warn",
		`href="/traces/4bf92f3577b34da6a3ce929d0e0e4736#span-00f067aa0ba902b7"`, // deep link to the span
		`href="/traces/abc"`, ">abc</a>", // short trace ID: no slicing panic, no span anchor
		"&lt;script&gt;alert(1)&lt;/script&gt;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("logs page missing %q", want)
		}
	}
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatal("log body rendered unescaped")
	}
	if strings.Contains(body, "not yet wired") {
		t.Error("placeholder text still shown")
	}
}

func TestLogsFilters(t *testing.T) {
	router, fs := logsRouter(t, sampleLogs())
	body := do(router, httptest.NewRequest(http.MethodGet, "/logs?min=warn&agent=bot&q=RETRY&window=1h", nil)).Body.String()
	lf := fs.lastLogFilter
	if lf.Window != time.Hour || lf.Agent != "bot" || lf.Search != "RETRY" || strings.Join(lf.Severities, ",") != "warn,error" {
		t.Errorf("filter = %+v", lf)
	}
	if !strings.Contains(body, "retrying tool call") || strings.Contains(body, "agent started") {
		t.Error("filtered rows wrong")
	}
	if !strings.Contains(body, `value="warn" selected`) || !strings.Contains(body, `href="/logs">Clear`) {
		t.Error("form should reflect the active filter")
	}

	body = do(router, httptest.NewRequest(http.MethodGet, "/logs?trace=abc", nil)).Body.String()
	if fs.lastLogFilter.TraceID != "abc" || !strings.Contains(body, "Showing logs for trace") {
		t.Error("trace filter not applied or not announced")
	}

	body = do(router, httptest.NewRequest(http.MethodGet, "/logs?agent=nobody", nil)).Body.String()
	if !strings.Contains(body, "No logs match these filters") {
		t.Error("empty filtered state missing")
	}
}

func TestParseLogListFilterRejectsBadInput(t *testing.T) {
	for q, want := range map[string]logListFilter{
		"min=fatal":                        {Window: "24h"},
		"min=warn%27--":                    {Window: "24h"},
		"trace=a%20b":                      {Window: "24h"},
		"trace=" + strings.Repeat("a", 65): {Window: "24h"},
		"q=" + strings.Repeat("x", 201):    {Window: "24h"},
		"q=%ff":                            {Window: "24h"},
		"window=1y":                        {Window: "24h"},
		"min=error&q=+timeout+&trace=abc":  {Window: "24h", MinSeverity: "error", Search: "timeout", TraceID: "abc"},
	} {
		v, _ := url.ParseQuery(q)
		if got := parseLogListFilter(v); got != want {
			t.Errorf("%s: got %+v, want %+v", q, got, want)
		}
	}
}

func TestLogsPartialAndTraceLink(t *testing.T) {
	router, _ := logsRouter(t, sampleLogs())
	req := httptest.NewRequest(http.MethodGet, "/logs?min=error", nil)
	req.Header.Set("HX-Request", "true")
	body := do(router, req).Body.String()
	if strings.Contains(body, "<html") || !strings.Contains(body, "logs-table") {
		t.Error("htmx request should get only the rows partial")
	}
	page := do(router, httptest.NewRequest(http.MethodGet, "/logs?min=error", nil)).Body.String()
	if !strings.Contains(page, `hx-get="/logs?min=error&amp;window=24h"`) {
		t.Error("polling should keep the filter")
	}

	r2, _ := listRouter(t, sampleRows())
	detail := do(r2, httptest.NewRequest(http.MethodGet, "/traces/aaaa", nil)).Body.String()
	if !strings.Contains(detail, `href="/logs?trace=aaaa&amp;window=7d"`) {
		t.Error("trace detail should link to its logs")
	}
}
