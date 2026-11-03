package health

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestLivezIgnoresChecks(t *testing.T) {
	h := Handler(map[string]Check{"db": func(context.Context) error { return errors.New("down") }}, logger())
	if code, body := get(t, h, "/livez"); code != 200 || body != "ok\n" {
		t.Fatalf("livez = %d %q", code, body)
	}
}

func TestReadyz(t *testing.T) {
	ok := func(context.Context) error { return nil }
	secret := errors.New("dial tcp clickhouse://obs:hunter2@10.0.0.5:9000: refused")
	fail := func(context.Context) error { return secret }

	code, body := get(t, Handler(map[string]Check{"nats": ok, "clickhouse": ok}, logger()), "/readyz")
	if code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("all ok: %d %s", code, body)
	}

	code, body = get(t, Handler(map[string]Check{"nats": ok, "clickhouse": fail}, logger()), "/readyz")
	var got struct {
		Status string
		Checks map[string]string
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if code != http.StatusServiceUnavailable || got.Status != "unavailable" ||
		got.Checks["clickhouse"] != "unavailable" || got.Checks["nats"] != "ok" {
		t.Fatalf("one failing: %d %+v", code, got)
	}
	if strings.Contains(body, "hunter2") || strings.Contains(body, "10.0.0.5") {
		t.Fatal("readyz leaked the check's error details")
	}
}

func TestReadyzTimesOutHungChecks(t *testing.T) {
	hung := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	start := time.Now()
	code, _ := get(t, Handler(map[string]Check{"hung": hung}, logger()), "/readyz")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("hung check: %d", code)
	}
	if d := time.Since(start); d > checkTimeout+time.Second {
		t.Fatalf("hung check took %v; should be bounded by %v", d, checkTimeout)
	}
}

func TestProbe(t *testing.T) {
	ready := true
	srv := httptest.NewServer(Handler(map[string]Check{"x": func(context.Context) error {
		if ready {
			return nil
		}
		return errors.New("no")
	}}, logger()))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	var out strings.Builder
	if code := Probe(":"+port, &out); code != 0 { // ":port" probes 127.0.0.1
		t.Fatalf("ready probe exit %d: %s", code, out.String())
	}
	ready = false
	if code := Probe("0.0.0.0:"+port, &out); code != 1 {
		t.Fatalf("unready probe exit %d", code)
	}
	if code := Probe("127.0.0.1:1", &out); code != 1 { // nothing listening
		t.Fatal("unreachable probe should fail")
	}
	if code := Probe("not-an-addr", &out); code != 1 {
		t.Fatal("bad addr should fail")
	}
}
