// Package health serves liveness and readiness endpoints on a plain-HTTP
// internal port, separate from a service's main listener (the gateway's gRPC
// port requires mTLS client certs, which probes can't present). It also
// provides the client side for the `healthcheck` subcommand that scratch
// images use for container health checks.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"time"
)

// Check reports whether one dependency is usable.
type Check func(ctx context.Context) error

// checkTimeout bounds each readiness check so a hung dependency can't hang the probe.
const checkTimeout = 2 * time.Second

// Handler serves GET /livez (process is up) and GET /readyz (every check passes).
//
// /readyz reports each check as "ok" or "unavailable". The underlying errors
// are logged rather than returned, so the endpoint never leaks DSNs or hostnames.
func Handler(checks map[string]Check, logger *slog.Logger) http.Handler {
	names := make([]string, 0, len(checks))
	for n := range checks {
		names = append(names, n)
	}
	sort.Strings(names)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		result := struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}{Status: "ok", Checks: make(map[string]string, len(names))}

		for _, name := range names {
			ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
			err := checks[name](ctx)
			cancel()
			if err != nil {
				result.Status = "unavailable"
				result.Checks[name] = "unavailable"
				logger.Warn("readiness check failed", "check", name, "err", err)
				continue
			}
			result.Checks[name] = "ok"
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if result.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	return mux
}

// Serve runs h on addr until ctx is done. Errors other than a clean shutdown are logged.
func Serve(ctx context.Context, addr string, h http.Handler, logger *slog.Logger) {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	logger.Info("health endpoints listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("health server", "err", err)
	}
}

// Probe GETs /readyz on the local health listener at addr (for example ":8081")
// and returns a process exit code: 0 when ready, 1 otherwise. It backs the
// `healthcheck` subcommand, since scratch images have no curl or wget.
func Probe(addr string, out io.Writer) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(out, "healthcheck: bad HEALTH_ADDR:", err)
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/readyz")
	if err != nil {
		fmt.Fprintln(out, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	fmt.Fprint(out, string(body))
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
