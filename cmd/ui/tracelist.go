package main

import (
	"html/template"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/your-org/agent-observability/internal/storage"
	conduitui "github.com/your-org/agent-observability/ui"
)

const (
	traceListLimit  = 100
	maxAgentNameLen = 256
)

// traceWindow is a selectable time range for the trace list. Only these
// values are accepted, so the query's scan is always bounded.
type traceWindow struct {
	Key   string
	Label string
	Dur   time.Duration
}

var traceWindows = []traceWindow{
	{"1h", "Last hour", time.Hour},
	{"24h", "Last 24 hours", 24 * time.Hour},
	{"7d", "Last 7 days", 7 * 24 * time.Hour},
}

const defaultTraceWindow = "24h"

// traceListFilter is the trace list's validated query string.
type traceListFilter struct {
	Window     string // a traceWindows key
	Agent      string
	ErrorsOnly bool
}

func parseTraceListFilter(q url.Values) traceListFilter {
	f := traceListFilter{Window: defaultTraceWindow}
	for _, w := range traceWindows {
		if q.Get("window") == w.Key {
			f.Window = w.Key
		}
	}
	// Agent names come from certificate CNs; reject anything that can't be one
	// rather than sending it to ClickHouse (it's a bound parameter either way).
	if a := q.Get("agent"); a != "" && len(a) <= maxAgentNameLen && utf8.ValidString(a) {
		f.Agent = a
	}
	f.ErrorsOnly = q.Get("errors") == "1"
	return f
}

func (f traceListFilter) storageFilter() storage.TraceFilter {
	sf := storage.TraceFilter{Agent: f.Agent, ErrorsOnly: f.ErrorsOnly, Limit: traceListLimit}
	for _, w := range traceWindows {
		if w.Key == f.Window {
			sf.Window = w.Dur
		}
	}
	return sf
}

// Query re-encodes the filter, so htmx polling keeps the current view.
func (f traceListFilter) Query() string {
	q := url.Values{"window": {f.Window}}
	if f.Agent != "" {
		q.Set("agent", f.Agent)
	}
	if f.ErrorsOnly {
		q.Set("errors", "1")
	}
	return q.Encode()
}

// Active reports whether any filter beyond the default window is applied.
func (f traceListFilter) Active() bool {
	return f.Agent != "" || f.ErrorsOnly || f.Window != defaultTraceWindow
}

// templateFuncs are available to every UI template.
var templateFuncs = template.FuncMap{
	"durationMs": func(start, end time.Time) int64 { return end.Sub(start).Milliseconds() },
}

// parseTemplates loads the embedded templates with templateFuncs.
func parseTemplates() *template.Template {
	return template.Must(template.New("").Funcs(templateFuncs).ParseFS(conduitui.Assets, "templates/*.html"))
}
