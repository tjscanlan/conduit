package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/your-org/agent-observability/internal/health"
	"github.com/your-org/agent-observability/internal/storage"
	"github.com/your-org/agent-observability/internal/telemetry"
)

func main() {
	cfg := loadConfig()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(health.Probe(cfg.HealthAddr, os.Stdout))
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// The collector owns the schema's retention; the UI only creates tables if missing.
	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN, storage.StoreOptions{
		RetentionDays: cfg.RetentionDays, ManageRetention: true,
	})
	if err != nil {
		logger.Error("clickhouse connect", "err", err)
		os.Exit(1)
	}

	vm := storage.NewVictoriaMetricsClient(cfg.VictoriaMetricsURL)
	tempo := storage.NewTempoClient(cfg.TempoEndpoint)
	exporter := telemetry.NewExporter(ch, vm, tempo, logger)

	collector, err := telemetry.NewCollector(cfg.NATSUrl, exporter, logger)
	if err != nil {
		logger.Error("collector init", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// VictoriaMetrics and Tempo are best-effort sinks, so only NATS and
	// ClickHouse (the system of record) gate readiness.
	go health.Serve(ctx, cfg.HealthAddr, health.Handler(map[string]health.Check{
		"nats":       collector.Ready,
		"clickhouse": ch.Ready,
	}, logger), logger)

	logger.Info("collector started", "nats", cfg.NATSUrl)
	if err := collector.Start(ctx); err != nil {
		logger.Error("collector stopped", "err", err)
	}
}
