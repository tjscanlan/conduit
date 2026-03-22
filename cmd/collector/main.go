package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/your-org/agent-observability/internal/storage"
	"github.com/your-org/agent-observability/internal/telemetry"
)

func main() {
	cfg := loadConfig()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ch, err := storage.NewClickHouseStore(cfg.ClickHouseDSN)
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

	logger.Info("collector started", "nats", cfg.NATSUrl)
	if err := collector.Start(ctx); err != nil {
		logger.Error("collector stopped", "err", err)
	}
}
