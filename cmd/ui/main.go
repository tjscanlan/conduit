package main

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/your-org/agent-observability/internal/auth"
	"github.com/your-org/agent-observability/internal/storage"
	conduitui "github.com/your-org/agent-observability/ui"
)

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN)
	if err != nil {
		logger.Error("clickhouse connect", "err", err)
		os.Exit(1)
	}

	tmpls := template.Must(template.ParseFS(conduitui.Assets, "templates/*.html"))

	h := &handlers{ch: ch, tmpls: tmpls, logger: logger}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServerFS(conduitui.Assets))
	mux.HandleFunc("/traces/{traceID}", h.traceDetail)
	mux.HandleFunc("/traces", h.traces)
	mux.HandleFunc("/logs", h.logs)
	mux.HandleFunc("/metrics", h.metricsPage)
	mux.HandleFunc("/", h.index)

	var root http.Handler = mux
	if cfg.APIToken != "" {
		store := auth.NewTokenStore()
		store.Add(cfg.APIToken, "ui-admin")
		root = auth.BearerTokenMiddleware(store, mux)
	}

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: root}

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
	ch     *storage.ClickHouseStore
	tmpls  *template.Template
	logger *slog.Logger
}

func (h *handlers) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/traces", http.StatusFound)
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
		_ = h.tmpls.ExecuteTemplate(w, "traces-rows", data)
		return
	}
	_ = h.tmpls.ExecuteTemplate(w, "traces", data)
}

func (h *handlers) traceDetail(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	spans, err := h.ch.QueryTrace(r.Context(), traceID)
	if err != nil {
		h.logger.Error("query trace", "traceID", traceID, "err", err)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	_ = h.tmpls.ExecuteTemplate(w, "traces", map[string]any{
		"Page":    "traces",
		"TraceID": traceID,
		"Spans":   spans,
	})
}

func (h *handlers) logs(w http.ResponseWriter, r *http.Request) {
	_ = h.tmpls.ExecuteTemplate(w, "logs", map[string]any{"Page": "logs"})
}

func (h *handlers) metricsPage(w http.ResponseWriter, r *http.Request) {
	_ = h.tmpls.ExecuteTemplate(w, "metrics", map[string]any{"Page": "metrics"})
}
