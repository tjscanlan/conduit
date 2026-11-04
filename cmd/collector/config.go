package main

import (
	"os"
	"strconv"

	"github.com/your-org/agent-observability/internal/storage"
)

type Config struct {
	NATSUrl            string
	ClickHouseDSN      string
	VictoriaMetricsURL string
	TempoEndpoint      string
	HealthAddr         string
	RetentionDays      int // SPAN_RETENTION_DAYS; validated by storage
}

func loadConfig() *Config {
	return &Config{
		NATSUrl:            getEnv("NATS_URL", "nats://localhost:4222"),
		ClickHouseDSN:      getEnv("CLICKHOUSE_DSN", "clickhouse://obs:secret@localhost:9000/observability"),
		VictoriaMetricsURL: getEnv("VICTORIAMETRICS_URL", "http://localhost:8428"),
		TempoEndpoint:      getEnv("TEMPO_ENDPOINT", "http://localhost:4318"), // OTLP/HTTP
		HealthAddr:         getEnv("HEALTH_ADDR", ":8081"),
		RetentionDays:      getEnvInt("SPAN_RETENTION_DAYS", storage.DefaultRetentionDays),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns -1 for a present but non-numeric value, so storage's
// range check rejects it instead of silently using the default.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}
