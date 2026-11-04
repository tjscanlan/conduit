package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjscanlan/conduit/internal/auth"
)

func quiet(t *testing.T) {
	t.Helper()
	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	t.Cleanup(func() { os.Stdout = old; devnull.Close() })
}

func TestInitIssuesAChainThatWorksForMTLS(t *testing.T) {
	quiet(t)
	dir := t.TempDir()
	if err := run(dir, []string{"demo-agent", "my-agent"}, []string{"gateway", "localhost", "127.0.0.1"}, "admin"); err != nil {
		t.Fatal(err)
	}
	c := func(n string) string { return filepath.Join(dir, "certs", n) }

	// A real TLS 1.3 handshake requiring client certs, using only the generated files.
	pool := x509.NewCertPool()
	caPEM, _ := os.ReadFile(c("ca.crt"))
	pool.AppendCertsFromPEM(caPEM)
	srv, err := tls.LoadX509KeyPair(c("server.crt"), c("server.key"))
	if err != nil {
		t.Fatal(err)
	}
	cli, err := tls.LoadX509KeyPair(c("my-agent.crt"), c("my-agent.key"))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{srv}, ClientCAs: pool,
		ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string)
	go func() { // report the client CN the server verified, per connection
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			tc := conn.(*tls.Conn)
			if err := tc.Handshake(); err != nil {
				got <- "handshake: " + err.Error()
			} else {
				got <- tc.ConnectionState().PeerCertificates[0].Subject.CommonName
			}
			_, _ = io.Copy(io.Discard, tc)
			tc.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	for _, host := range []string{"localhost", "127.0.0.1"} { // both SANs must verify
		conn, err := tls.Dial("tcp", net.JoinHostPort(host, port), &tls.Config{Certificates: []tls.Certificate{cli}, RootCAs: pool, MinVersion: tls.VersionTLS13})
		if err != nil {
			t.Fatalf("dial %s: %v", host, err)
		}
		_, _ = conn.Write([]byte("x")) // completes the server-side handshake
		if cn := <-got; cn != "my-agent" {
			t.Fatalf("server saw %q, want agent CN my-agent", cn)
		}
		conn.Close()
	}
}

func TestInitUILoginIsUsableAndIdempotent(t *testing.T) {
	quiet(t)
	dir := t.TempDir()
	if err := run(dir, []string{"demo-agent"}, []string{"gateway"}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.LoadUsersFile(filepath.Join(dir, "ui-users")); err != nil {
		t.Fatalf("ui-users must parse with the UI's own loader: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "certs", "demo-agent.crt"))
	beforeUsers, _ := os.ReadFile(filepath.Join(dir, "ui-users"))

	// Second run: everything kept (restarts reuse credentials).
	if err := run(dir, []string{"demo-agent"}, []string{"gateway"}, "admin"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "certs", "demo-agent.crt"))
	afterUsers, _ := os.ReadFile(filepath.Join(dir, "ui-users"))
	if string(before) != string(after) || string(beforeUsers) != string(afterUsers) {
		t.Error("rerun must keep existing certificates and the UI login")
	}

	// Adding an agent later issues only that one.
	if err := run(dir, []string{"demo-agent", "new-bot"}, []string{"gateway"}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "certs", "new-bot.crt")); err != nil {
		t.Error("new agent cert not issued")
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "certs", "demo-agent.crt")); string(again) != string(before) {
		t.Error("existing agent cert reissued unnecessarily")
	}
}

func TestInitRejectsBadAgentNames(t *testing.T) {
	quiet(t)
	for _, bad := range []string{"../etc", "a b", strings.Repeat("x", 65)} {
		if err := run(t.TempDir(), []string{bad}, []string{"gateway"}, "admin"); err == nil {
			t.Errorf("agent name %q accepted", bad)
		}
	}
}

func TestInitSecretsAreOwnerOnlyAndLoginMatches(t *testing.T) {
	quiet(t)
	dir := t.TempDir()
	if err := run(dir, []string{"my-agent"}, []string{"gateway"}, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"certs/ca.key", "certs/server.key", "certs/my-agent.key", "ui-login.txt"} {
		info, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s has mode %o, want 600", f, perm)
		}
	}
	login, _ := os.ReadFile(filepath.Join(dir, "ui-login.txt"))
	user, pass, ok := strings.Cut(strings.TrimSpace(string(login)), " ")
	if !ok || user != "admin" || len(pass) < 20 {
		t.Fatalf("ui-login.txt = %q, want \"admin <password>\"", login)
	}
	users, err := auth.LoadUsersFile(filepath.Join(dir, "ui-users"))
	if err != nil {
		t.Fatal(err)
	}
	if !users.Authenticate(user, pass) || users.Authenticate(user, pass+"x") {
		t.Error("ui-login.txt password must (only) match the ui-users hash")
	}
}
