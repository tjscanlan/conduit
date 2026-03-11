package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// AgentIdentity is extracted from the client's mTLS certificate
type AgentIdentity struct {
	CommonName   string
	Organization []string
	DNSNames     []string
}

// ExtractIdentity pulls the agent identity from the verified peer certificate.
// This is called after TLS handshake — the cert is already verified by Go's TLS stack.
func ExtractIdentity(ctx context.Context) (*AgentIdentity, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("no peer in context")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, errors.New("peer has no TLS info — connection not mTLS")
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return nil, errors.New("no verified certificate chain")
	}

	leaf := tlsInfo.State.VerifiedChains[0][0]
	return &AgentIdentity{
		CommonName:   leaf.Subject.CommonName,
		Organization: leaf.Subject.Organization,
		DNSNames:     leaf.DNSNames,
	}, nil
}

// AuthInterceptor is a gRPC unary interceptor that enforces mTLS identity extraction
func AuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		identity, err := ExtractIdentity(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "identity extraction failed: %v", err)
		}
		ctx = context.WithValue(ctx, agentIdentityKey{}, identity)
		return handler(ctx, req)
	}
}

type agentIdentityKey struct{}

// IdentityFromContext retrieves the agent identity set by AuthInterceptor
func IdentityFromContext(ctx context.Context) (*AgentIdentity, bool) {
	id, ok := ctx.Value(agentIdentityKey{}).(*AgentIdentity)
	return id, ok
}

// LoadClientTLSConfig builds a tls.Config for agent SDKs connecting to the gateway
func LoadClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
