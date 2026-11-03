package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	conduitui "github.com/your-org/agent-observability/ui"
)

func TestSecurityHeadersAppliedEvenOnAuthFailure(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/traces", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect to login", rec.Code)
	}
	want := map[string]string{
		"Content-Security-Policy": contentSecurityPolicy,
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// The CSP forbids inline script; guard against templates reintroducing it.
func TestTemplatesHaveNoInlineScript(t *testing.T) {
	inlineHandler := regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	inlineScript := regexp.MustCompile(`(?i)<script(\s[^>]*)?>\s*[^<\s]`)
	inlineStyle := regexp.MustCompile(`(?i)<style|\sstyle\s*=`)

	paths, err := fs.Glob(conduitui.Assets, "templates/*.html")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no templates found: %v", err)
	}
	for _, p := range paths {
		b, err := fs.ReadFile(conduitui.Assets, p)
		if err != nil {
			t.Fatal(err)
		}
		for name, re := range map[string]*regexp.Regexp{
			"inline event handler": inlineHandler,
			"inline script":        inlineScript,
			"inline style":         inlineStyle,
		} {
			if loc := re.FindIndex(b); loc != nil {
				t.Errorf("%s: %s at byte %d: %q", p, name, loc[0], b[loc[0]:min(loc[1]+20, len(b))])
			}
		}
	}
	// Templates must still parse.
	parseTemplates()
}
