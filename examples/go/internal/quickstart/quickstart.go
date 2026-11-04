// Package quickstart points the SDK at the local quickstart unless the
// environment says otherwise.
//
// The SDK reads CONDUIT_GATEWAY_ADDR, CONDUIT_CERT_DIR and CONDUIT_AGENT
// (see sdk.NewFromEnv). When CONDUIT_CERT_DIR isn't set, this fills in the
// certificates `make quickstart` generated (searching upward from the
// current directory), as agent "my-agent". CONDUIT_UI_URL sets where trace
// links point (default http://localhost:8080).
package quickstart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	sdk "github.com/tjscanlan/conduit/sdk/go"
)

// Client returns a client from the environment, after checking that the
// gateway accepts it, so a setup mistake fails here with a hint.
func Client() (*sdk.Client, error) {
	if os.Getenv(sdk.EnvCertDir) == "" {
		if dir := findQuickstartCerts(); dir != "" {
			os.Setenv(sdk.EnvCertDir, dir)
			if os.Getenv(sdk.EnvAgent) == "" {
				os.Setenv(sdk.EnvAgent, "my-agent")
			}
		}
	}
	c, err := sdk.NewFromEnv() // your own code needs only this line
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Check(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// TraceURL is where a trace can be seen in the Conduit UI.
func TraceURL(traceID string) string {
	ui := os.Getenv("CONDUIT_UI_URL")
	if ui == "" {
		ui = "http://localhost:8080"
	}
	return ui + "/traces/" + traceID
}

func findQuickstartCerts() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "deployments", "quickstart", ".conduit", "certs")
		if _, err := os.Stat(filepath.Join(candidate, "ca.crt")); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Fatal prints err and exits.
func Fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
