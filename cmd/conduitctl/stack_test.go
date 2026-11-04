package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjscanlan/conduit/internal/devpki"
)

func TestWriteBundleAndEnv(t *testing.T) {
	s := &stack{dir: t.TempDir(), out: &bytes.Buffer{}}
	if err := s.writeBundle(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"compose.yml", "tempo.yaml", "clickhouse/low-memory.xml", "clickhouse/low-memory-users.xml"} {
		if _, err := os.Stat(filepath.Join(s.dir, f)); err != nil {
			t.Errorf("bundle missing %s", f)
		}
	}
	compose, _ := os.ReadFile(filepath.Join(s.dir, "compose.yml"))
	if strings.Contains(string(compose), "build:") || strings.Contains(string(compose), "../") {
		t.Error("the embedded compose.yml must only pull images and reference files inside the bundle")
	}

	if err := s.writeEnv(); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(s.dir, ".env")
	first, _ := os.ReadFile(env)
	if info, _ := os.Stat(env); info.Mode().Perm() != 0o600 {
		t.Errorf(".env mode = %o, want 600", info.Mode().Perm())
	}
	if !strings.Contains(string(first), "CLICKHOUSE_PASSWORD=") || !strings.Contains(string(first), "UI_SESSION_KEY=") {
		t.Errorf(".env = %q", first)
	}
	_ = s.writeEnv()
	if again, _ := os.ReadFile(env); string(again) != string(first) {
		t.Error("writeEnv must keep existing secrets")
	}
}

func TestDevBuildNeedsAnImageTag(t *testing.T) {
	s := &stack{images: "dev"}
	if err := s.checkImages(); err == nil || !strings.Contains(err.Error(), "-images") {
		t.Fatalf("err = %v", err)
	}
	s.images = "v0.2.0"
	if err := s.checkImages(); err != nil {
		t.Fatal(err)
	}
}

func TestIsPseudoVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.2.0": false, "v0.2.0-rc.1": false,
		"v0.0.0-20261104223000-abcdef123456": true, "v0.2.1-0.20261104223000-abcdef123456": true,
	} {
		if got := isPseudoVersion(v); got != want {
			t.Errorf("isPseudoVersion(%q) = %v", v, got)
		}
	}
}

func TestAgentCertAndBanner(t *testing.T) {
	out := &bytes.Buffer{}
	s := &stack{dir: t.TempDir(), out: out, uiPort: 18080, gatewayPort: 14317}
	if err := s.agentCert("bot"); err == nil {
		t.Fatal("agent-cert before the quickstart ran must fail")
	}
	if err := devpki.Init(filepath.Join(s.dir, ".conduit"), []string{"my-agent"}, []string{"gateway"}, "admin", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := s.agentCert("research-bot"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CONDUIT_AGENT=research-bot") {
		t.Errorf("agent-cert output: %s", out)
	}
	out.Reset()
	s.banner()
	for _, want := range []string{"http://localhost:18080", "admin / ", "CONDUIT_GATEWAY_ADDR=localhost:14317",
		"CONDUIT_CERT_DIR=" + s.certDir(), "conduitctl agent-cert"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("banner missing %q", want)
		}
	}
	if strings.Contains(out.String(), "PRIVATE KEY") {
		t.Error("banner must not print key material")
	}
}
