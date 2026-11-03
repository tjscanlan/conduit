package main

import (
	"os"
	"strconv"
)

type Config struct {
	ListenAddr  string
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
	NATSUrl     string
	RateLimit   int // telemetry items (spans, metrics, logs) per second per agent identity
	HealthAddr  string
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:  getEnv("GATEWAY_ADDR", ":4317"),
		TLSCertFile: getEnv("TLS_CERT", "/etc/certs/server.crt"),
		TLSKeyFile:  getEnv("TLS_KEY", "/etc/certs/server.key"),
		TLSCAFile:   getEnv("TLS_CA", "/etc/certs/ca.crt"),
		NATSUrl:     getEnv("NATS_URL", "nats://localhost:4222"),
		RateLimit:   getEnvPositiveInt("RATE_LIMIT_RPS", 100),
		HealthAddr:  getEnv("HEALTH_ADDR", ":8081"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvPositiveInt falls back when the value is unset, malformed, or not
// positive. A zero rate limit would otherwise silently reject every request.
func getEnvPositiveInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			return i
		}
	}
	return fallback
}
