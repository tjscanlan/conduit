package main

import "os"

type Config struct {
	ListenAddr    string
	ClickHouseDSN string
	APIToken      string // if set, require Bearer auth on all routes
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
		ClickHouseDSN: getEnv("CLICKHOUSE_DSN", "clickhouse://obs:secret@localhost:9000/observability"),
		APIToken:      getEnv("UI_API_TOKEN", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
