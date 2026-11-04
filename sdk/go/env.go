package sdk

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Environment variables read by NewFromEnv and ConfigFromEnv.
const (
	EnvGatewayAddr = "CONDUIT_GATEWAY_ADDR" // default localhost:4317
	EnvCertDir     = "CONDUIT_CERT_DIR"     // directory with ca.crt and <agent>.crt/.key
	EnvAgent       = "CONDUIT_AGENT"        // which <agent>.crt in CONDUIT_CERT_DIR
	EnvTLSCert     = "CONDUIT_TLS_CERT"     // explicit paths; override CONDUIT_CERT_DIR
	EnvTLSKey      = "CONDUIT_TLS_KEY"
	EnvTLSCA       = "CONDUIT_TLS_CA"

	// DefaultGatewayAddr is the local quickstart's gateway.
	DefaultGatewayAddr = "localhost:4317"
)

// NewFromEnv creates a client configured by the environment. The shortest
// setup is a certificate directory, such as the one `make quickstart`
// writes:
//
//	CONDUIT_CERT_DIR=deployments/quickstart/.conduit/certs CONDUIT_AGENT=my-agent
//
// It uses <dir>/ca.crt and <dir>/<agent>.crt/.key. CONDUIT_AGENT may be
// omitted when the directory holds exactly one client certificate.
// CONDUIT_TLS_CERT, CONDUIT_TLS_KEY and CONDUIT_TLS_CA set the files
// directly, and CONDUIT_GATEWAY_ADDR the gateway (default localhost:4317).
func NewFromEnv() (*Client, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// ConfigFromEnv resolves a Config from the environment without connecting;
// see NewFromEnv for the variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{GatewayAddr: firstEnv(EnvGatewayAddr, "OTEL_GATEWAY_ADDR")}
	if cfg.GatewayAddr == "" {
		cfg.GatewayAddr = DefaultGatewayAddr
	}
	cfg.TLSCert, cfg.TLSKey, cfg.TLSCACert = os.Getenv(EnvTLSCert), os.Getenv(EnvTLSKey), os.Getenv(EnvTLSCA)

	if dir := os.Getenv(EnvCertDir); dir != "" {
		agent := os.Getenv(EnvAgent)
		if agent == "" && (cfg.TLSCert == "" || cfg.TLSKey == "") {
			var err error
			if agent, err = soleClientCert(dir); err != nil {
				return Config{}, err
			}
		}
		if cfg.TLSCert == "" {
			cfg.TLSCert = filepath.Join(dir, agent+".crt")
		}
		if cfg.TLSKey == "" {
			cfg.TLSKey = filepath.Join(dir, agent+".key")
		}
		if cfg.TLSCACert == "" {
			cfg.TLSCACert = filepath.Join(dir, "ca.crt")
		}
	}

	var missing []string
	for name, v := range map[string]string{EnvTLSCert: cfg.TLSCert, EnvTLSKey: cfg.TLSKey, EnvTLSCA: cfg.TLSCACert} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Config{}, fmt.Errorf("conduit: no agent certificate configured: set %s (and %s if it holds several agents), "+
			"or %s; for a local stack run `make quickstart` and use its deployments/quickstart/.conduit/certs",
			EnvCertDir, EnvAgent, strings.Join(missing, ", "))
	}
	return cfg, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// soleClientCert returns the name of the only client certificate in dir.
func soleClientCert(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.crt"))
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("conduit: %s=%s has no certificates (is the path right?)", EnvCertDir, dir)
	}
	var agents []string
	for _, p := range paths {
		if isClientCert(p) {
			agents = append(agents, strings.TrimSuffix(filepath.Base(p), ".crt"))
		}
	}
	switch len(agents) {
	case 1:
		return agents[0], nil
	case 0:
		return "", fmt.Errorf("conduit: %s=%s has no client certificates; create one with `scripts/gen-certs.sh agent NAME`", EnvCertDir, dir)
	default:
		return "", fmt.Errorf("conduit: %s=%s has several agent certificates (%s): set %s to pick one",
			EnvCertDir, dir, strings.Join(agents, ", "), EnvAgent)
	}
}

func isClientCert(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.IsCA {
		return false
	}
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageClientAuth {
			return true
		}
	}
	return false
}
