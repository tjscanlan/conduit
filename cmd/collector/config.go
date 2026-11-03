package main

import "os"

type Config struct {
	NATSUrl            string
	ClickHouseDSN      string
	VictoriaMetricsURL string
	TempoEndpoint      string
	HealthAddr         string
}

func loadConfig() *Config {
	return &Config{
		NATSUrl:            getEnv("NATS_URL", "nats://localhost:4222"),
		ClickHouseDSN:      getEnv("CLICKHOUSE_DSN", "clickhouse://obs:secret@localhost:9000/observability"),
		VictoriaMetricsURL: getEnv("VICTORIAMETRICS_URL", "http://localhost:8428"),
		TempoEndpoint:      getEnv("TEMPO_ENDPOINT", "http://localhost:4318"), // OTLP/HTTP
		HealthAddr:         getEnv("HEALTH_ADDR", ":8081"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
