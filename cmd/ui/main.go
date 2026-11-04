package main

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tjscanlan/conduit/internal/health"
	"github.com/tjscanlan/conduit/internal/storage"
	conduitui "github.com/tjscanlan/conduit/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		os.Exit(hashPassword(os.Stdin, os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(health.Probe(getEnv("LISTEN_ADDR", ":8080"), os.Stdout))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid config", "err", err)
		os.Exit(1)
	}

	tmpls := parseTemplates()

	// Set up auth before touching storage so misconfiguration fails fast.
	authn, err := newUIAuth(cfg, tmpls, logger)
	if err != nil {
		logger.Error("ui auth setup", "mode", cfg.AuthMode, "err", err)
		os.Exit(1)
	}

	// Read-only: the collector owns the schema (migrations and retention).
	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN, storage.StoreOptions{})
	if err != nil {
		logger.Error("clickhouse connect", "err", err)
		os.Exit(1)
	}

	h := &handlers{ch: ch, tmpls: tmpls, logger: logger}
	root := newRouter(authn, h)
	logger.Info("ui auth configured", "mode", cfg.AuthMode)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("ui listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down ui")
	_ = srv.Shutdown(context.Background())
}

type handlers struct {
	ch     spanQuerier
	tmpls  *template.Template
	logger *slog.Logger
}

// spanQuerier is the read side of storage.ClickHouseStore used by the UI.
type spanQuerier interface {
	QueryTraces(ctx context.Context, f storage.TraceFilter) ([]storage.TraceSummary, error)
	QueryAgents(ctx context.Context, window time.Duration, limit int) ([]string, error)
	Ready(ctx context.Context) error
	QueryLogs(ctx context.Context, f storage.LogFilter) ([]storage.LogRow, error)
	QueryTrace(ctx context.Context, traceID string, limit int) ([]storage.SpanRow, error)
}

// newRouter builds the UI's handler tree. Only /healthz, /static/, and the
// login endpoints are reachable without authentication.
func newRouter(authn *uiAuth, h *handlers) http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("/traces/{traceID}", h.traceDetail)
	protected.HandleFunc("/traces", h.traces)
	protected.HandleFunc("/logs", h.logs)
	protected.HandleFunc("/metrics", h.metricsPage)
	protected.HandleFunc("/", h.index)

	mux := http.NewServeMux()
	// Load balancer health checks (including GCP's, which bypass IAP) hit this directly.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	// Readiness: can the UI query ClickHouse? Unauthenticated like /healthz,
	// and it reports only ok/unavailable.
	mux.Handle("GET /readyz", health.Handler(map[string]health.Check{"clickhouse": h.ch.Ready}, h.logger))
	mux.Handle("/static/", http.FileServerFS(conduitui.Assets))
	authn.register(mux, protected)
	return securityHeaders(mux)
}

// render executes a template, adding the signed-in user for the nav bar.
func (h *handlers) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	data["User"] = uiUserFromContext(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpls.ExecuteTemplate(w, name, data); err != nil {
		h.logger.Error("render", "template", name, "err", err)
	}
}

func (h *handlers) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, defaultLandingPath, http.StatusFound)
}

func (h *handlers) traces(w http.ResponseWriter, r *http.Request) {
	f := parseTraceListFilter(r.URL.Query())
	traces, err := h.ch.QueryTraces(r.Context(), f.storageFilter())
	if err != nil {
		h.logger.Error("query traces", "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Page": "traces", "Filter": f, "Windows": traceWindows}
	// Agent suggestions are a convenience: the htmx partial doesn't need them,
	// and a failure only loses the suggestions, not the page.
	if r.Header.Get("HX-Request") != "true" {
		agents, err := h.ch.QueryAgents(r.Context(), f.storageFilter().Window, maxAgentSuggestions)
		if err != nil {
			h.logger.Warn("query agents", "err", err)
		}
		data["Agents"] = agents
	}
	if len(traces) > traceListLimit {
		traces = traces[:traceListLimit]
		data["OlderURL"] = f.OlderURL(traces[len(traces)-1])
	}
	data["Traces"] = traces
	if r.Header.Get("HX-Request") == "true" {
		h.render(w, r, "traces-rows", data)
		return
	}
	h.render(w, r, "traces", data)
}

func (h *handlers) traceDetail(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	// Fetch one extra row to tell whether the trace was truncated.
	spans, err := h.ch.QueryTrace(r.Context(), traceID, maxTraceSpans+1)
	if err != nil {
		h.logger.Error("query trace", "traceID", traceID, "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if len(spans) == 0 {
		http.NotFound(w, r)
		return
	}
	truncated := len(spans) > maxTraceSpans
	if truncated {
		spans = spans[:maxTraceSpans]
	}
	view := buildTraceView(traceID, spans)
	view.Truncated = truncated
	h.render(w, r, "trace", map[string]any{
		"Page":  "traces",
		"Trace": view,
	})
}

func (h *handlers) logs(w http.ResponseWriter, r *http.Request) {
	f := parseLogListFilter(r.URL.Query())
	logs, err := h.ch.QueryLogs(r.Context(), f.storageFilter())
	if err != nil {
		h.logger.Error("query logs", "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Page": "logs", "Logs": logs, "Filter": f, "Windows": traceWindows,
		"Levels": severityLevels, "Limit": logListLimit, "Full": len(logs) == logListLimit}
	if r.Header.Get("HX-Request") == "true" {
		h.render(w, r, "logs-rows", data)
		return
	}
	h.render(w, r, "logs", data)
}

func (h *handlers) metricsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "metrics", map[string]any{"Page": "metrics"})
}
