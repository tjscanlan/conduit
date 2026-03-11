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
	RateLimit   int // requests per second per agent identity
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:  getEnv("GATEWAY_ADDR", ":4317"),
		TLSCertFile: getEnv("TLS_CERT", "/etc/certs/server.crt"),
		TLSKeyFile:  getEnv("TLS_KEY", "/etc/certs/server.key"),
		TLSCAFile:   getEnv("TLS_CA", "/etc/certs/ca.crt"),
		NATSUrl:     getEnv("NATS_URL", "nats://localhost:4222"),
		RateLimit:   getEnvInt("RATE_LIMIT_RPS", 100),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
