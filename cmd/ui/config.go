package main

import (
	"fmt"
	"os"
	"time"
)

// Auth modes for the UI.
const (
	authModeLogin = "login" // login form + signed session cookie
	authModeIAP   = "iap"   // Google Cloud Identity-Aware Proxy JWT
	authModeNone  = "none"  // no auth; local development only
)

type Config struct {
	ListenAddr    string
	ClickHouseDSN string

	AuthMode string

	// login mode
	UsersFile      string        // htpasswd-style "user:bcrypt-hash" lines
	SessionKey     string        // HMAC key, >= 32 bytes; random per process if empty
	SessionTTL     time.Duration //
	CookieInsecure bool          // allow session cookie over plain HTTP (dev only)

	// iap mode
	IAPAudience string // e.g. /projects/123/global/backendServices/456

	// Get started page
	GatewayAddr string // the gateway address agents should use (display only)
	DemoAgent   string // the sample agent whose traces don't count as "yours"
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		ClickHouseDSN:  getEnv("CLICKHOUSE_DSN", "clickhouse://obs:secret@localhost:9000/observability"),
		AuthMode:       getEnv("UI_AUTH_MODE", authModeLogin),
		UsersFile:      os.Getenv("UI_USERS_FILE"),
		SessionKey:     os.Getenv("UI_SESSION_KEY"),
		CookieInsecure: os.Getenv("UI_COOKIE_INSECURE") == "true",
		IAPAudience:    os.Getenv("UI_IAP_AUDIENCE"),
		GatewayAddr:    getEnv("UI_GATEWAY_ADDR", "localhost:4317"),
		DemoAgent:      getEnv("UI_DEMO_AGENT", "demo-agent"),
	}
	ttl, err := time.ParseDuration(getEnv("UI_SESSION_TTL", "8h"))
	if err != nil || ttl <= 0 {
		return nil, fmt.Errorf("UI_SESSION_TTL: invalid duration %q", os.Getenv("UI_SESSION_TTL"))
	}
	cfg.SessionTTL = ttl

	switch cfg.AuthMode {
	case authModeLogin:
		if cfg.UsersFile == "" {
			return nil, fmt.Errorf("UI_AUTH_MODE=login requires UI_USERS_FILE")
		}
		if cfg.SessionKey != "" && len(cfg.SessionKey) < 32 {
			return nil, fmt.Errorf("UI_SESSION_KEY must be at least 32 bytes")
		}
	case authModeIAP:
		if cfg.IAPAudience == "" {
			return nil, fmt.Errorf("UI_AUTH_MODE=iap requires UI_IAP_AUDIENCE")
		}
	case authModeNone:
	default:
		return nil, fmt.Errorf("UI_AUTH_MODE: unknown mode %q (want login, iap, or none)", cfg.AuthMode)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
