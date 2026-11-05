package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

func onboardingRouter(t *testing.T, rows []storage.SpanRow) (http.Handler, *fakeSpans) {
	t.Helper()
	tmpls := parseTemplates()
	a, err := newUIAuth(&Config{AuthMode: authModeNone}, tmpls, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeSpans{rows: rows}
	h := &handlers{ch: fs, tmpls: tmpls, logger: testLogger(), gatewayAddr: "localhost:4317", demoAgent: "demo-agent"}
	return newRouter(a, h), fs
}

func demoRows() []storage.SpanRow {
	return []storage.SpanRow{{TraceID: "d1", SpanID: "s1", SpanName: "agent.run", AgentName: "demo-agent", Status: "ok", StartTime: time.Now()}}
}

func get(t *testing.T, h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return do(h, req)
}

func TestNewInstallLandsOnGetStarted(t *testing.T) {
	for name, rows := range map[string][]storage.SpanRow{"no data": nil, "only the demo agent": demoRows()} {
		t.Run(name, func(t *testing.T) {
			h, _ := onboardingRouter(t, rows)
			if rec := get(t, h, "/"); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/start" {
				t.Fatalf("/ -> %d %q, want 302 /start", rec.Code, rec.Header().Get("Location"))
			}
			body := get(t, h, "/start").Body.String()
			for _, want := range []string{
				"Get started", "Conduit is running", "CONDUIT_GATEWAY_ADDR=localhost:4317",
				"CONDUIT_AGENT=my-agent", "scripts/gen-certs.sh agent", "never shown or served by this UI",
				`hx-get="/start/status"`, "Waiting for a trace from your agent", `class="nav-start active"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("/start missing %q", want)
				}
			}
			if strings.Contains(body, "PRIVATE KEY") || strings.Contains(body, "BEGIN ") {
				t.Error("/start must never contain key material")
			}
			if rows != nil && !strings.Contains(body, "is sending runs") {
				t.Error("/start should say the demo agent is sending data")
			}
			// Other pages offer Get started in the nav until onboarded.
			if !strings.Contains(get(t, h, "/traces").Body.String(), `href="/start"`) {
				t.Error("/traces nav should link to Get started")
			}
		})
	}
}

func TestFirstTraceFromYourAgentCompletesOnboarding(t *testing.T) {
	rows := append(demoRows(), storage.SpanRow{TraceID: "abc123", SpanID: "s9", SpanName: "hello.run", AgentName: "my-agent", Status: "ok", StartTime: time.Now()})
	h, fs := onboardingRouter(t, rows)

	status := get(t, h, "/start/status")
	body := status.Body.String()
	for _, want := range []string{"Your agent is connected", "my-agent", `href="/traces/abc123"`} {
		if !strings.Contains(body, want) {
			t.Errorf("status fragment missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "hx-trigger") {
		t.Error("status fragment must stop polling once the first trace arrived")
	}
	if strings.Contains(body, "<nav") {
		t.Error("the htmx fragment must not render the nav")
	}

	if rec := get(t, h, "/"); rec.Header().Get("Location") != defaultLandingPath {
		t.Fatalf("/ -> %q, want %s once onboarded", rec.Header().Get("Location"), defaultLandingPath)
	}
	if strings.Contains(get(t, h, "/traces").Body.String(), `href="/start"`) {
		t.Error("nav should drop Get started once onboarded")
	}
	// Once onboarded, pages stop querying for it.
	calls := fs.onboardingCalls
	for range 5 {
		get(t, h, "/traces")
	}
	if fs.onboardingCalls != calls {
		t.Errorf("onboarding queried %d more times after it completed", fs.onboardingCalls-calls)
	}
}

func TestDismissGetStarted(t *testing.T) {
	h, _ := onboardingRouter(t, nil)

	cross := httptest.NewRequest(http.MethodPost, "/start/dismiss", nil)
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := do(h, cross); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site dismiss = %d, want 403", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/start/dismiss", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := do(h, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("dismiss = %d", rec.Code)
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == onboardingCookie {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("dismiss cookie = %+v", cookie)
	}
	if loc := get(t, h, "/", cookie).Header().Get("Location"); loc != defaultLandingPath {
		t.Errorf("after dismiss / -> %q", loc)
	}
	if strings.Contains(get(t, h, "/traces", cookie).Body.String(), `href="/start"`) {
		t.Error("nav should hide Get started after dismiss")
	}
	if get(t, h, "/start", cookie).Code != http.StatusOK {
		t.Error("/start must stay reachable after dismiss")
	}
}

func TestGetStartedWhenQueriesFail(t *testing.T) {
	h, fs := onboardingRouter(t, nil)
	fs.onboardingErr = errors.New("clickhouse down")
	fs.readyErr = errors.New("clickhouse down")
	rec := get(t, h, "/start")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"can't reach ClickHouse", "Couldn't query telemetry", "retrying"} {
		if !strings.Contains(body, want) {
			t.Errorf("/start missing %q", want)
		}
	}
}

// The live status box and the landing redirect must agree right away, even
// while the not-yet-onboarded answer is cached.
func TestStatusSeeingTheFirstTraceUpdatesLandingAtOnce(t *testing.T) {
	h, fs := onboardingRouter(t, demoRows())
	if loc := get(t, h, "/").Header().Get("Location"); loc != "/start" {
		t.Fatalf("/ -> %q before any real trace", loc)
	}
	fs.rows = append(fs.rows, storage.SpanRow{TraceID: "t1", SpanID: "x", SpanName: "run", AgentName: "my-agent", Status: "ok", StartTime: time.Now()})
	if !strings.Contains(get(t, h, "/start/status").Body.String(), "Your agent is connected") {
		t.Fatal("status should report the new trace")
	}
	if loc := get(t, h, "/").Header().Get("Location"); loc != defaultLandingPath {
		t.Fatalf("/ -> %q right after the status saw the trace, want %s", loc, defaultLandingPath)
	}
}
