package main

import (
	"html/template"
	"net/url"
	"strconv"
	"strings"
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
	Before     string // keyset cursor "<startedUnixNano>.<traceID>"; empty for the newest page
}

// traceCursor encodes the position after t for the "older" link.
func traceCursor(t storage.TraceSummary) string {
	return strconv.FormatInt(t.Started.UnixNano(), 10) + "." + t.TraceID
}

// parseTraceCursor validates a cursor; IDs follow schema.validID's charset.
func parseTraceCursor(c string) (time.Time, string, bool) {
	ns, id, ok := strings.Cut(c, ".")
	if !ok || id == "" || len(id) > 64 {
		return time.Time{}, "", false
	}
	n, err := strconv.ParseInt(ns, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, "", false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_') {
			return time.Time{}, "", false
		}
	}
	return time.Unix(0, n).UTC(), id, true
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
	if _, _, ok := parseTraceCursor(q.Get("before")); ok {
		f.Before = q.Get("before")
	}
	return f
}

func (f traceListFilter) storageFilter() storage.TraceFilter {
	// One extra row tells us whether there is an older page.
	sf := storage.TraceFilter{Agent: f.Agent, ErrorsOnly: f.ErrorsOnly, Limit: traceListLimit + 1}
	if t, id, ok := parseTraceCursor(f.Before); ok {
		sf.BeforeStarted, sf.BeforeTraceID = t, id
	}
	for _, w := range traceWindows {
		if w.Key == f.Window {
			sf.Window = w.Dur
		}
	}
	return sf
}

// Query re-encodes the filter (including the cursor), so htmx polling and
// links keep the current view.
func (f traceListFilter) Query() string {
	q := f.values()
	if f.Before != "" {
		q.Set("before", f.Before)
	}
	return q.Encode()
}

// OlderURL links to the page after the given last row. It is a complete,
// already-encoded URL: interpolating a query string into href="/traces?{{.}}"
// would make html/template percent-encode its = and & and drop the cursor.
func (f traceListFilter) OlderURL(last storage.TraceSummary) template.URL {
	q := f.values()
	q.Set("before", traceCursor(last))
	return template.URL("/traces?" + q.Encode()) // url.Values.Encode escapes every value
}

// NewestURL drops the cursor, returning to the first page.
func (f traceListFilter) NewestURL() template.URL {
	return template.URL("/traces?" + f.values().Encode())
}

func (f traceListFilter) values() url.Values {
	q := url.Values{"window": {f.Window}}
	if f.Agent != "" {
		q.Set("agent", f.Agent)
	}
	if f.ErrorsOnly {
		q.Set("errors", "1")
	}
	return q
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
