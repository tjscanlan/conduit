package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr  string
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
	NATSUrl     string
	RateLimit   int // telemetry items (spans, metrics, logs) per second per agent identity
	HealthAddr  string
	// StoreLLMPayloads keeps prompts and completions (STORE_LLM_PAYLOADS,
	// default true). Model, token counts, and cost are always kept.
	StoreLLMPayloads bool
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:       getEnv("GATEWAY_ADDR", ":4317"),
		TLSCertFile:      getEnv("TLS_CERT", "/etc/certs/server.crt"),
		TLSKeyFile:       getEnv("TLS_KEY", "/etc/certs/server.key"),
		TLSCAFile:        getEnv("TLS_CA", "/etc/certs/ca.crt"),
		NATSUrl:          getEnv("NATS_URL", "nats://localhost:4222"),
		RateLimit:        getEnvPositiveInt("RATE_LIMIT_RPS", 100),
		HealthAddr:       getEnv("HEALTH_ADDR", ":8081"),
		StoreLLMPayloads: getEnvBool("STORE_LLM_PAYLOADS", true),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvBool reads true/false, 1/0, yes/no, on/off (any case); unset uses
// fallback. Unrecognized values are reported by validateConfig, so a typo in
// a privacy setting stops startup instead of silently using the default.
func getEnvBool(key string, fallback bool) bool {
	v, ok := parseBool(os.Getenv(key))
	if !ok {
		return fallback
	}
	return v
}

func parseBool(s string) (value, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	}
	return false, false
}

// validateConfig reports settings that are present but unparseable.
func validateConfig() error {
	if v := os.Getenv("STORE_LLM_PAYLOADS"); v != "" {
		if _, ok := parseBool(v); !ok {
			return fmt.Errorf("STORE_LLM_PAYLOADS=%q: want true or false", v)
		}
	}
	return nil
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
