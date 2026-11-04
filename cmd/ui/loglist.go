package main

import (
	"html/template"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/storage"
)

const (
	logListLimit     = 200
	maxLogSearchLen  = 200
	maxLogTraceIDLen = 64
)

// severityLevels orders the filterable severities; "min" selects that level and above.
var severityLevels = []string{schema.SeverityDebug, schema.SeverityInfo, schema.SeverityWarn, schema.SeverityError}

// logListFilter is the logs page's validated query string.
type logListFilter struct {
	Window      string // a traceWindows key
	Agent       string
	MinSeverity string // "" = all, including unspecified
	Search      string
	TraceID     string
}

func parseLogListFilter(q url.Values) logListFilter {
	f := logListFilter{Window: parseTraceListFilter(q).Window} // same allowlisted windows
	if a := q.Get("agent"); a != "" && len(a) <= maxAgentNameLen && utf8.ValidString(a) {
		f.Agent = a
	}
	for _, lvl := range severityLevels {
		if q.Get("min") == lvl {
			f.MinSeverity = lvl
		}
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" && len(s) <= maxLogSearchLen && utf8.ValidString(s) {
		f.Search = s
	}
	if t := q.Get("trace"); t != "" && len(t) <= maxLogTraceIDLen && isIDChars(t) {
		f.TraceID = t
	}
	return f
}

func isIDChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (f logListFilter) storageFilter() storage.LogFilter {
	sf := storage.LogFilter{Agent: f.Agent, Search: f.Search, TraceID: f.TraceID, Limit: logListLimit}
	for _, w := range traceWindows {
		if w.Key == f.Window {
			sf.Window = w.Dur
		}
	}
	if f.MinSeverity != "" {
		on := false
		for _, lvl := range severityLevels {
			on = on || lvl == f.MinSeverity
			if on {
				sf.Severities = append(sf.Severities, lvl)
			}
		}
	}
	return sf
}

// URL is the logs page for this filter (complete and pre-encoded; see OlderURL).
func (f logListFilter) URL() template.URL {
	q := url.Values{"window": {f.Window}}
	for k, v := range map[string]string{"agent": f.Agent, "min": f.MinSeverity, "q": f.Search, "trace": f.TraceID} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return template.URL("/logs?" + q.Encode())
}

// Active reports whether any filter beyond the default window is applied.
func (f logListFilter) Active() bool {
	return f.Agent != "" || f.MinSeverity != "" || f.Search != "" || f.TraceID != "" || f.Window != defaultTraceWindow
}
