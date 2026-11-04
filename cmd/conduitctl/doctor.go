package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/tjscanlan/conduit/proto"
)

const minDockerMemory = 2 << 30

// doctor runs each check, printing ok/FAIL with a fix, and fails if any did.
func (s *stack) doctor() error {
	failed := 0
	check := func(name string, fn func() (string, error)) bool {
		detail, err := fn()
		if err != nil {
			failed++
			fmt.Fprintf(s.out, "FAIL  %s\n      %v\n", name, err)
			return false
		}
		fmt.Fprintf(s.out, "ok    %s%s\n", name, suffix(detail))
		return true
	}

	if !check("Docker is installed and running, with Compose v2", func() (string, error) { return "", preflight() }) {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	check("Docker has enough memory", dockerMemory)

	running := s.runningServices()
	if len(running) == 0 {
		check("UI and gateway ports are free", func() (string, error) { return "", s.portsFree() })
		fmt.Fprintln(s.out, "--    the stack is not running: start it with `conduitctl quickstart`")
		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	}

	check("every service is running and healthy", func() (string, error) { return s.servicesHealthy(running) })
	check("the UI answers", s.uiReady)
	check("the gateway accepts my-agent's certificate", s.gatewayAccepts)
	check("demo-agent data reached ClickHouse", func() (string, error) {
		if _, ok := running["demo-agent"]; !ok {
			return "demo-agent isn't running (fine if you started with -no-demo)", nil
		}
		if !s.demoTraceSeen() {
			return "", fmt.Errorf("no demo-agent spans yet; see `docker compose -p %s logs demo-agent gateway collector`", projectName)
		}
		return "", nil
	})
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	fmt.Fprintln(s.out, "\nEverything looks healthy.")
	return nil
}

func suffix(detail string) string {
	if detail == "" {
		return ""
	}
	return " (" + detail + ")"
}

func dockerMemory() (string, error) {
	out, err := exec.Command("docker", "info", "--format", "{{.MemTotal}}").Output()
	if err != nil {
		return "", fmt.Errorf("docker info: %w", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return "", nil // unknown; don't fail on it
	}
	gib := fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	if n < minDockerMemory*95/100 {
		return "", fmt.Errorf("Docker has %s; give it at least 2 GiB (Docker Desktop: Settings → Resources; Colima: colima start --memory 2)", gib)
	}
	return gib, nil
}

func (s *stack) portsFree() error {
	for _, p := range []struct {
		name string
		port int
		flag string
	}{{"UI", s.uiPort, "-ui-port"}, {"gateway", s.gatewayPort, "-gateway-port"}} {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.port)))
		if err != nil {
			return fmt.Errorf("port %d (%s) is in use by another program; pick another with %s", p.port, p.name, p.flag)
		}
		ln.Close()
	}
	return nil
}

type composeService struct {
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

// runningServices lists the project's containers by service (nil if none).
func (s *stack) runningServices() map[string]composeService {
	out, err := exec.Command("docker", "compose", "-p", projectName, "ps", "-a", "--format", "json").Output()
	if err != nil {
		return nil
	}
	svcs := map[string]composeService{}
	// Compose prints one JSON object per line (older versions: one array).
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}
		var list []composeService
		if json.Unmarshal(raw, &list) != nil {
			var one composeService
			if json.Unmarshal(raw, &one) != nil {
				continue
			}
			list = []composeService{one}
		}
		for _, c := range list {
			svcs[c.Service] = c
		}
	}
	if len(svcs) == 0 {
		return nil
	}
	return svcs
}

func (s *stack) servicesHealthy(svcs map[string]composeService) (string, error) {
	var bad []string
	for name, c := range svcs {
		switch {
		case name == "conduit-init":
			if c.State != "exited" {
				bad = append(bad, name+" is "+c.State)
			}
		case c.State != "running":
			bad = append(bad, name+" is "+c.State)
		case c.Health != "" && c.Health != "healthy":
			bad = append(bad, name+" is "+c.Health)
		}
	}
	if len(bad) > 0 {
		return "", fmt.Errorf("%s; see `docker compose -p %s logs %s`", strings.Join(bad, ", "), projectName, firstWord(bad))
	}
	return fmt.Sprintf("%d services", len(svcs)), nil
}

func firstWord(items []string) string { return strings.Fields(items[0])[0] }

func (s *stack) uiReady() (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", s.uiPort)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("%s: %v; is another program on port %d, or did the stack start with other ports?", url, err, s.uiPort)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %d: %s (the UI can't reach ClickHouse; see `docker compose -p %s logs ui clickhouse`)",
			url, resp.StatusCode, strings.TrimSpace(string(body)), projectName)
	}
	return fmt.Sprintf("http://localhost:%d", s.uiPort), nil
}

// gatewayAccepts makes the call an agent would, as my-agent: an empty batch
// over mTLS. It proves the certificates chain and the gateway is serving.
func (s *stack) gatewayAccepts() (string, error) {
	dir := s.certDir()
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "my-agent.crt"), filepath.Join(dir, "my-agent.key"))
	if err != nil {
		return "", fmt.Errorf("load my-agent's certificate from %s: %w", dir, err)
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return "", err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	addr := net.JoinHostPort("localhost", strconv.Itoa(s.gatewayPort))
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert}, RootCAs: pool, MinVersion: tls.VersionTLS13,
	})))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pb.NewIngestionServiceClient(conn).IngestBatch(ctx, &pb.IngestBatchRequest{}); err != nil {
		return "", fmt.Errorf("%s: %v; see `docker compose -p %s logs gateway`", addr, err, projectName)
	}
	return addr, nil
}
