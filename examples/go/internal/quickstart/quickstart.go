// Package quickstart connects the examples to a Conduit gateway with as
// little setup as possible: by default it finds the certificates that
// `make quickstart` generated and sends as agent "my-agent".
//
// Environment:
//
//	CONDUIT_GATEWAY_ADDR  gateway address (default localhost:4317)
//	CONDUIT_CERT_DIR      directory with ca.crt and <agent>.crt/.key
//	                      (default: deployments/quickstart/.conduit/certs,
//	                      searched for from the current directory upward)
//	CONDUIT_AGENT         which agent certificate to use (default my-agent)
//	CONDUIT_UI_URL        where to link traces (default http://localhost:8080)
package quickstart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sdk "github.com/tjscanlan/conduit/sdk/go"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Client returns an SDK client configured from the environment.
func Client() (*sdk.Client, error) {
	dir := os.Getenv("CONDUIT_CERT_DIR")
	if dir == "" {
		var err error
		if dir, err = findCertDir(); err != nil {
			return nil, err
		}
	}
	agent := env("CONDUIT_AGENT", "my-agent")
	return sdk.New(sdk.Config{
		GatewayAddr: env("CONDUIT_GATEWAY_ADDR", "localhost:4317"),
		TLSCert:     filepath.Join(dir, agent+".crt"),
		TLSKey:      filepath.Join(dir, agent+".key"),
		TLSCACert:   filepath.Join(dir, "ca.crt"),
	})
}

// TraceURL is where a trace can be seen in the Conduit UI.
func TraceURL(traceID string) string {
	return env("CONDUIT_UI_URL", "http://localhost:8080") + "/traces/" + traceID
}

func findCertDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "deployments", "quickstart", ".conduit", "certs")
		if _, err := os.Stat(filepath.Join(candidate, "ca.crt")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no quickstart certificates found: run `make quickstart` in the repo root, or set CONDUIT_CERT_DIR")
		}
		dir = parent
	}
}

// Fatal prints err with a hint and exits.
func Fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
