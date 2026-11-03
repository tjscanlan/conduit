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

	"github.com/your-org/agent-observability/internal/storage"
	conduitui "github.com/your-org/agent-observability/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		os.Exit(hashPassword(os.Stdin, os.Stdout, os.Stderr))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid config", "err", err)
		os.Exit(1)
	}

	tmpls := template.Must(template.ParseFS(conduitui.Assets, "templates/*.html"))

	// Set up auth before touching storage so misconfiguration fails fast.
	authn, err := newUIAuth(cfg, tmpls, logger)
	if err != nil {
		logger.Error("ui auth setup", "mode", cfg.AuthMode, "err", err)
		os.Exit(1)
	}

	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN)
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
	QueryRecentSpans(ctx context.Context, limit int) ([]storage.SpanRow, error)
	QueryTrace(ctx context.Context, traceID string) ([]storage.SpanRow, error)
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
	rows, err := h.ch.QueryRecentSpans(r.Context(), 100)
	if err != nil {
		h.logger.Error("query spans", "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Page": "traces", "Rows": rows}
	if r.Header.Get("HX-Request") == "true" {
		h.render(w, r, "traces-rows", data)
		return
	}
	h.render(w, r, "traces", data)
}

func (h *handlers) traceDetail(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	spans, err := h.ch.QueryTrace(r.Context(), traceID)
	if err != nil {
		h.logger.Error("query trace", "traceID", traceID, "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if len(spans) == 0 {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "trace", map[string]any{
		"Page":  "traces",
		"Trace": buildTraceView(traceID, spans),
	})
}

func (h *handlers) logs(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "logs", map[string]any{"Page": "logs"})
}

func (h *handlers) metricsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "metrics", map[string]any{"Page": "metrics"})
}
