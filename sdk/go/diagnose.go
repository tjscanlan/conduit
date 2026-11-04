package sdk

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/tjscanlan/conduit/proto"
)

// Check verifies that the gateway is reachable and accepts this agent's
// certificate, by sending an empty batch. Call it at startup to fail fast
// with an actionable error instead of on the first span.
func (c *Client) Check(ctx context.Context) error {
	if _, err := c.ingest.IngestBatch(ctx, &pb.IngestBatchRequest{}); err != nil {
		return c.diagnose("check gateway", err)
	}
	return nil
}

// diagnose wraps an RPC error with a hint about the likely cause. The
// original error stays in the chain for errors.Is / status.FromError.
func (c *Client) diagnose(op string, err error) error {
	if hint := c.hint(err); hint != "" {
		return fmt.Errorf("%s: %s: %w", op, hint, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func (c *Client) hint(err error) string {
	msg := err.Error()
	st, _ := status.FromError(err)
	switch {
	case strings.Contains(msg, "certificate signed by unknown authority"):
		return fmt.Sprintf("the gateway's certificate is not signed by the CA in %s; use the ca.crt issued with the gateway's certificate", c.cfg.TLSCACert)
	case strings.Contains(msg, "certificate is valid for"), strings.Contains(msg, "doesn't contain any IP SANs"):
		return fmt.Sprintf("the gateway's certificate does not cover the host in %q; connect using a name in its certificate (e.g. localhost)", c.cfg.GatewayAddr)
	case strings.Contains(msg, "bad certificate"), strings.Contains(msg, "unknown certificate authority"),
		strings.Contains(msg, "certificate required"), strings.Contains(msg, "unknown certificate"):
		return fmt.Sprintf("the gateway rejected this agent's certificate %s; it must be signed by the CA the gateway trusts (the gateway's TLS_CA)", c.cfg.TLSCert)
	case strings.Contains(msg, "certificate has expired"), strings.Contains(msg, "expired certificate"):
		return "a certificate has expired; issue a new one (scripts/gen-certs.sh, or `quickstart.sh reset` for the quickstart)"
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"):
		return fmt.Sprintf("can't reach the gateway at %s; is it running? (for a local stack: make quickstart)", c.cfg.GatewayAddr)
	case strings.Contains(msg, "first record does not look like a TLS handshake"):
		return fmt.Sprintf("%s is not speaking TLS; check %s points at the gateway's gRPC port", c.cfg.GatewayAddr, EnvGatewayAddr)
	}
	rejected := fmt.Sprintf("the gateway rejected this agent's certificate %s; it must be signed by the CA the gateway trusts (the gateway's TLS_CA)", c.cfg.TLSCert)
	if c.certFromOtherCA && st.Code() == codes.Unavailable {
		// TLS 1.3 reports a rejected client certificate after the handshake,
		// often as a bare broken connection.
		return fmt.Sprintf("%s, and it is not signed by %s", rejected, c.cfg.TLSCACert)
	}
	switch st.Code() {
	case codes.Unauthenticated, codes.PermissionDenied:
		return "the gateway did not accept this agent's identity; check the client certificate"
	case codes.ResourceExhausted:
		return "the gateway is rate limiting this agent (RATE_LIMIT_RPS items/s per agent); send less often or batch"
	case codes.DeadlineExceeded:
		return fmt.Sprintf("the gateway at %s did not answer in time", c.cfg.GatewayAddr)
	case codes.Unavailable:
		return fmt.Sprintf("the gateway at %s is unavailable; is it running and is the address right?", c.cfg.GatewayAddr)
	}
	return ""
}

// checkClientCert catches certificate mistakes when the client is created,
// rather than as a TLS failure on the first span.
func checkClientCert(path string, cert *x509.Certificate) error {
	now := time.Now()
	switch {
	case now.After(cert.NotAfter):
		return fmt.Errorf("client certificate %s expired on %s; issue a new one", path, cert.NotAfter.Format(time.DateOnly))
	case now.Before(cert.NotBefore):
		return fmt.Errorf("client certificate %s is not valid until %s (is the clock right?)", path, cert.NotBefore.Format(time.RFC3339))
	case cert.Subject.CommonName == "":
		return fmt.Errorf("client certificate %s has no CommonName; the gateway uses it as the agent's name", path)
	}
	if len(cert.ExtKeyUsage) > 0 {
		for _, u := range cert.ExtKeyUsage {
			if u == x509.ExtKeyUsageClientAuth || u == x509.ExtKeyUsageAny {
				return nil
			}
		}
		return errors.New("certificate " + path + " is not a client certificate (no clientAuth usage); is it the gateway's server.crt? Use an agent certificate")
	}
	return nil
}
