package sdk

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// certDir lays pki out the way `make quickstart` does: ca.crt, <agent>.crt/.key.
func certDir(t *testing.T, pki *testPKI, agents ...string) string {
	t.Helper()
	dir := t.TempDir()
	cp := func(src, dst string) {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, dst), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cp(pki.caFile, "ca.crt")
	for _, a := range agents {
		cp(pki.certFile, a+".crt")
		cp(pki.keyFile, a+".key")
	}
	return dir
}

func clearEnv(t *testing.T) {
	for _, k := range []string{EnvGatewayAddr, "OTEL_GATEWAY_ADDR", EnvCertDir, EnvAgent, EnvTLSCert, EnvTLSKey, EnvTLSCA} {
		t.Setenv(k, "")
	}
}

func TestConfigFromEnvCertDir(t *testing.T) {
	pki := newTestPKI(t, t.TempDir(), "conduit")

	t.Run("sole agent is picked automatically", func(t *testing.T) {
		clearEnv(t)
		dir := certDir(t, pki, "my-agent")
		t.Setenv(EnvCertDir, dir)
		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.GatewayAddr != DefaultGatewayAddr || cfg.TLSCert != filepath.Join(dir, "my-agent.crt") ||
			cfg.TLSKey != filepath.Join(dir, "my-agent.key") || cfg.TLSCACert != filepath.Join(dir, "ca.crt") {
			t.Errorf("cfg = %+v", cfg)
		}
	})
	t.Run("several agents need CONDUIT_AGENT", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(EnvCertDir, certDir(t, pki, "demo-agent", "my-agent"))
		_, err := ConfigFromEnv()
		if err == nil || !strings.Contains(err.Error(), "demo-agent, my-agent") || !strings.Contains(err.Error(), EnvAgent) {
			t.Fatalf("err = %v, want it to list the agents and name %s", err, EnvAgent)
		}
		t.Setenv(EnvAgent, "my-agent")
		t.Setenv(EnvGatewayAddr, "gw:1234")
		cfg, err := ConfigFromEnv()
		if err != nil || !strings.HasSuffix(cfg.TLSCert, "my-agent.crt") || cfg.GatewayAddr != "gw:1234" {
			t.Fatalf("cfg = %+v, err = %v", cfg, err)
		}
	})
	t.Run("explicit files override the directory", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(EnvCertDir, certDir(t, pki, "a", "b"))
		t.Setenv(EnvTLSCert, "/x.crt")
		t.Setenv(EnvTLSKey, "/x.key")
		cfg, err := ConfigFromEnv()
		if err != nil || cfg.TLSCert != "/x.crt" || !strings.HasSuffix(cfg.TLSCACert, "ca.crt") {
			t.Fatalf("cfg = %+v, err = %v", cfg, err)
		}
	})
	t.Run("nothing set explains what to do", func(t *testing.T) {
		clearEnv(t)
		_, err := ConfigFromEnv()
		if err == nil || !strings.Contains(err.Error(), EnvCertDir) || !strings.Contains(err.Error(), "make quickstart") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestNewFromEnvSendsSpans(t *testing.T) {
	clearEnv(t)
	pki := newTestPKI(t, t.TempDir(), "conduit")
	g, addr := startGateway(t, pki)
	t.Setenv(EnvCertDir, certDir(t, pki, "my-agent"))
	t.Setenv(EnvGatewayAddr, addr)
	c, err := NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}
	s, _ := c.StartSpan(ctx, "from-env")
	if err := s.End(ctx); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.spans) != 1 {
		t.Fatalf("gateway got %d spans", len(g.spans))
	}
}

// Each first-run mistake gets an error that says what to do.
func TestDiagnostics(t *testing.T) {
	dir := t.TempDir()
	pki := newTestPKI(t, dir, "gateway")
	other := newTestPKI(t, dir, "other")
	_, addr := startGateway(t, pki)

	check := func(t *testing.T, cfg Config, want string) {
		t.Helper()
		c, err := New(cfg)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = c.Check(ctx)
			c.Close()
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v\nwant it to contain %q", err, want)
		}
		t.Log(err)
	}
	good := Config{GatewayAddr: addr, TLSCert: pki.certFile, TLSKey: pki.keyFile, TLSCACert: pki.caFile}

	t.Run("wrong CA for the gateway", func(t *testing.T) {
		cfg := good
		cfg.TLSCACert = other.caFile
		check(t, cfg, "gateway's certificate is not signed by the CA in "+other.caFile)
	})
	t.Run("agent cert from a CA the gateway doesn't trust", func(t *testing.T) {
		cfg := good
		cfg.TLSCert, cfg.TLSKey = other.certFile, other.keyFile
		check(t, cfg, "the gateway rejected this agent's certificate")
	})
	t.Run("gateway not running", func(t *testing.T) {
		cfg := good
		cfg.GatewayAddr = "localhost:1"
		check(t, cfg, "can't reach the gateway at localhost:1")
	})
	t.Run("host not in the gateway's certificate", func(t *testing.T) {
		cfg := good
		cfg.GatewayAddr = strings.Replace(addr, "localhost", "127.0.0.1", 1)
		check(t, cfg, "does not cover the host")
	})
	t.Run("missing certificate file", func(t *testing.T) {
		cfg := good
		cfg.TLSCert = filepath.Join(dir, "nope.crt")
		check(t, cfg, "make quickstart")
	})
	t.Run("key from another certificate", func(t *testing.T) {
		cfg := good
		cfg.TLSKey = other.keyFile
		check(t, cfg, "the key belongs to a different certificate")
	})
	t.Run("the gateway's server certificate used as an agent", func(t *testing.T) {
		srvCrt, srvKey := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
		writeServerPair(t, pki, srvCrt, srvKey)
		cfg := good
		cfg.TLSCert, cfg.TLSKey = srvCrt, srvKey
		check(t, cfg, "not a client certificate")
	})
}

func writeServerPair(t *testing.T, pki *testPKI, crtPath, keyPath string) {
	t.Helper()
	kb, err := x509.MarshalECPrivateKey(pki.serverCert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pki.serverCert.Certificate[0]}), 0o600)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
}
