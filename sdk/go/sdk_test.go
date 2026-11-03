package sdk

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/your-org/agent-observability/proto"
)

// testPKI is a throwaway CA with server and client certificates on disk.
type testPKI struct {
	caPool                    *x509.CertPool
	serverCert                tls.Certificate
	caFile, certFile, keyFile string
}

func newTestPKI(t *testing.T, dir, name string) *testPKI {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name + " CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	leaf := func(cn string, usage x509.ExtKeyUsage, dns []string) ([]byte, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			DNSNames:     dns,
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return der, key
	}
	pemCert := func(der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}) }
	pemKey := func(k *ecdsa.PrivateKey) []byte {
		b, _ := x509.MarshalECPrivateKey(k)
		return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	}

	srvDER, srvKey := leaf("gateway", x509.ExtKeyUsageServerAuth, []string{"localhost"})
	serverCert, err := tls.X509KeyPair(pemCert(srvDER), pemKey(srvKey))
	if err != nil {
		t.Fatal(err)
	}
	cliDER, cliKey := leaf("agent-1", x509.ExtKeyUsageClientAuth, nil)

	p := &testPKI{
		caPool:     x509.NewCertPool(),
		serverCert: serverCert,
		caFile:     filepath.Join(dir, name+"-ca.crt"),
		certFile:   filepath.Join(dir, name+"-client.crt"),
		keyFile:    filepath.Join(dir, name+"-client.key"),
	}
	p.caPool.AddCert(caCert)
	for path, data := range map[string][]byte{
		p.caFile:   pemCert(caDER),
		p.certFile: pemCert(cliDER),
		p.keyFile:  pemKey(cliKey),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// fakeGateway records spans and rejects any whose name is "reject-me".
type fakeGateway struct {
	pb.UnimplementedIngestionServiceServer
	mu    sync.Mutex
	spans []*pb.Span
}

func (g *fakeGateway) IngestSpan(_ context.Context, req *pb.IngestSpanRequest) (*pb.IngestSpanResponse, error) {
	if req.GetSpan().GetName() == "reject-me" {
		return &pb.IngestSpanResponse{Accepted: false, RejectionReason: "span name is not allowed"}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.spans = append(g.spans, req.GetSpan())
	return &pb.IngestSpanResponse{Accepted: true}, nil
}

// startGateway serves the fake gateway over mTLS, requiring client certs from pki's CA.
func startGateway(t *testing.T, pki *testPKI) (*fakeGateway, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{pki.serverCert},
		ClientCAs:    pki.caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	})
	srv := grpc.NewServer(grpc.Creds(creds))
	g := &fakeGateway{}
	pb.RegisterIngestionServiceServer(srv, g)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	_, port, _ := net.SplitHostPort(lis.Addr().String())
	return g, "localhost:" + port
}

func newTestClient(t *testing.T, addr string, pki *testPKI) *Client {
	t.Helper()
	c, err := New(Config{
		GatewayAddr: addr, AgentName: "agent-1",
		TLSCert: pki.certFile, TLSKey: pki.keyFile, TLSCACert: pki.caFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestSpansReachGatewayOverMTLS(t *testing.T) {
	pki := newTestPKI(t, t.TempDir(), "conduit")
	g, addr := startGateway(t, pki)
	c := newTestClient(t, addr, pki)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root, ctx := c.StartSpan(ctx, "agent.run")
	root.SetAttr("task", "summarize")
	child, _ := c.StartSpan(ctx, "llm.call")
	child.RecordLLMCall(LLMCall{Model: "m", Prompt: "p", Completion: "c", InputTokens: 3, OutputTokens: 5, CostUSD: 0.25})

	if err := child.End(ctx); err != nil {
		t.Fatalf("child.End: %v", err)
	}
	if err := root.End(ctx); err != nil {
		t.Fatalf("root.End: %v", err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.spans) != 2 {
		t.Fatalf("gateway received %d spans, want 2", len(g.spans))
	}
	gotChild, gotRoot := g.spans[0], g.spans[1]
	if gotRoot.GetName() != "agent.run" || gotRoot.GetAttributes()["task"] != "summarize" || gotRoot.GetParentId() != "" {
		t.Errorf("root span = %v", gotRoot)
	}
	if gotChild.GetTraceId() != gotRoot.GetTraceId() || gotChild.GetParentId() != gotRoot.GetSpanId() {
		t.Errorf("child not linked to root: trace %q/%q parent %q root span %q",
			gotChild.GetTraceId(), gotRoot.GetTraceId(), gotChild.GetParentId(), gotRoot.GetSpanId())
	}
	llm := gotChild.GetLlmCall()
	if llm.GetModel() != "m" || llm.GetInputTokens() != 3 || llm.GetOutputTokens() != 5 || llm.GetCostUsd() != 0.25 {
		t.Errorf("llm call = %v", llm)
	}
	if gotChild.GetEndTime().AsTime().Before(gotChild.GetStartTime().AsTime()) {
		t.Errorf("end before start: %v", gotChild)
	}
}

func TestEndReportsGatewayRejection(t *testing.T) {
	pki := newTestPKI(t, t.TempDir(), "conduit")
	_, addr := startGateway(t, pki)
	c := newTestClient(t, addr, pki)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, _ := c.StartSpan(ctx, "reject-me")
	err := s.End(ctx)
	if err == nil || !strings.Contains(err.Error(), "span name is not allowed") {
		t.Fatalf("End = %v, want rejection reason", err)
	}
}

// The SDK must trust only the configured CA: a gateway whose certificate
// chains to a different CA is refused even though the client cert is fine.
func TestRejectsGatewayFromUntrustedCA(t *testing.T) {
	dir := t.TempDir()
	gatewayPKI := newTestPKI(t, dir, "gateway")
	otherPKI := newTestPKI(t, dir, "other")
	g, addr := startGateway(t, gatewayPKI)

	c, err := New(Config{
		GatewayAddr: addr,
		TLSCert:     gatewayPKI.certFile, TLSKey: gatewayPKI.keyFile,
		TLSCACert: otherPKI.caFile, // wrong CA for the server
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, _ := c.StartSpan(ctx, "x")
	if err := s.End(ctx); err == nil {
		t.Fatal("span delivered to a gateway signed by an untrusted CA")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.spans) != 0 {
		t.Fatal("gateway received a span over an untrusted connection")
	}
}

func TestNewValidatesCA(t *testing.T) {
	pki := newTestPKI(t, t.TempDir(), "conduit")
	bad := filepath.Join(t.TempDir(), "bad.crt")
	_ = os.WriteFile(bad, []byte("not a pem"), 0o600)

	for name, ca := range map[string]string{"missing": "/nonexistent/ca.crt", "not PEM": bad} {
		if _, err := New(Config{GatewayAddr: "localhost:1", TLSCert: pki.certFile, TLSKey: pki.keyFile, TLSCACert: ca}); err == nil {
			t.Errorf("%s CA: New succeeded, want error", name)
		}
	}
}
