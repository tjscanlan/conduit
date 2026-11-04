package main

import (
	"context"
	"fmt"
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
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		// Apply migrations and reconcile retention, then exit (e.g. a k8s Job
		// before rolling out collectors). NewClickHouseStore does the work.
		if _, err := storage.NewClickHouseStore(cfg.ClickHouseDSN, storage.StoreOptions{
			RetentionDays: cfg.RetentionDays, ManageSchema: true,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", err)
			os.Exit(1)
		}
		fmt.Println("schema up to date")
		os.Exit(0)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// The collector owns the schema: it applies migrations, then reconciles retention.
	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN, storage.StoreOptions{
		RetentionDays: cfg.RetentionDays, ManageSchema: true,
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
