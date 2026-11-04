// Package devpki generates the local quickstart's credentials: a
// development CA, the gateway's server certificate, agent client
// certificates, and a UI login. Everything lives in one directory and is
// idempotent: valid existing files are kept, so restarts reuse credentials.
//
// Development use only: the CA key lives next to the certificates it signs.
package devpki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 365 * 24 * time.Hour
)

var agentName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidAgentName reports whether name can be an agent's certificate name
// (it becomes a file name and the certificate's CommonName).
func ValidAgentName(name string) bool { return agentName.MatchString(name) }

// SplitList splits a comma-separated list, dropping empty items.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// layout is where conduit-init writes, relative to the output directory.
type layout struct{ dir string }

func (l layout) certs() string        { return filepath.Join(l.dir, "certs") }
func (l layout) path(p string) string { return filepath.Join(l.dir, p) }

// Init writes the dev CA, the gateway certificate, a client certificate per
// agent, and a UI login into dir, keeping valid existing files. It reports
// what it did to out.
func Init(dir string, agents, gatewayHosts []string, uiUser string, out io.Writer) error {
	for _, a := range agents {
		if !agentName.MatchString(a) {
			return fmt.Errorf("agent name %q must be 1-64 chars of [A-Za-z0-9._-]", a)
		}
	}
	l := layout{dir}
	if err := os.MkdirAll(l.certs(), 0o755); err != nil {
		return err
	}

	caCert, caKey, created, err := loadOrCreateCA(l)
	if err != nil {
		return err
	}
	report := func(what string, made bool) {
		state := "kept existing"
		if made {
			state = "created"
		}
		fmt.Fprintf(out, "  %-34s %s\n", what, state)
	}
	fmt.Fprintln(out, "conduit-init: preparing quickstart credentials")
	report("certs/ca.crt", created)

	made, err := ensureLeaf(l, "server", "gateway", gatewayHosts, x509.ExtKeyUsageServerAuth, caCert, caKey, created)
	if err != nil {
		return err
	}
	report("certs/server.crt (gateway)", made)
	for _, a := range agents {
		made, err := ensureLeaf(l, a, a, nil, x509.ExtKeyUsageClientAuth, caCert, caKey, created)
		if err != nil {
			return err
		}
		report(fmt.Sprintf("certs/%s.crt (agent)", a), made)
	}

	password, made, err := ensureUILogin(l, uiUser)
	if err != nil {
		return err
	}
	report("ui-users (UI login)", made)
	if made {
		fmt.Fprintf(out, "\nUI login: %s / %s\n", uiUser, password)
	}
	return nil
}

// loadOrCreateCA reuses an existing CA, or creates one. created reports a new
// CA, in which case every leaf must be reissued so it chains to it.
func loadOrCreateCA(l layout) (*x509.Certificate, *ecdsa.PrivateKey, bool, error) {
	crtPath, keyPath := filepath.Join(l.certs(), "ca.crt"), filepath.Join(l.certs(), "ca.key")
	if cert, key, err := readPair(crtPath, keyPath); err == nil {
		return cert, key, false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, false, fmt.Errorf("existing CA is unreadable (remove the volume to start over): %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "Conduit quickstart CA", Organization: []string{"Conduit (development)"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, false, err
	}
	cert, _ := x509.ParseCertificate(der)
	if err := writePair(crtPath, keyPath, der, key); err != nil {
		return nil, nil, false, err
	}
	return cert, key, true, nil
}

// ensureLeaf issues name.crt/name.key unless a valid pair from this CA exists.
func ensureLeaf(l layout, name, cn string, hosts []string, usage x509.ExtKeyUsage, ca *x509.Certificate, caKey *ecdsa.PrivateKey, force bool) (bool, error) {
	crtPath, keyPath := filepath.Join(l.certs(), name+".crt"), filepath.Join(l.certs(), name+".key")
	if !force {
		if cert, _, err := readPair(crtPath, keyPath); err == nil && cert.CheckSignatureFrom(ca) == nil && time.Now().Before(cert.NotAfter) {
			return false, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Conduit (development)"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return false, err
	}
	return true, writePair(crtPath, keyPath, der, key)
}

// ensureUILogin writes ui-users ("user:bcrypt-hash") with a random password,
// unless the file already exists. The plain password is also kept in
// ui-login.txt (owner-only) so quickstart.sh can show it again on later runs.
func ensureUILogin(l layout, user string) (string, bool, error) {
	path := l.path("ui-users")
	if _, err := os.Stat(path); err == nil {
		return "", false, nil
	}
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	password := base64.RawURLEncoding.EncodeToString(buf) // 20 chars, ~120 bits
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(l.path("ui-login.txt"), []byte(user+" "+password+"\n"), 0o600); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(user+":"+string(hash)+"\n"), 0o644); err != nil {
		return "", false, err
	}
	return password, true, nil
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

// Keys are owner-only. The quickstart runs conduit-init as the host user so
// the agent keys are usable from the host; the services run as root in their
// scratch images and can read them. This is for local development, not
// production (see docs/deployment.md for real PKI).
func writePair(crtPath, keyPath string, der []byte, key *ecdsa.PrivateKey) error {
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

func readPair(crtPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cb, err := os.ReadFile(crtPath)
	if err != nil {
		return nil, nil, err
	}
	kb, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	cp, _ := pem.Decode(cb)
	kp, _ := pem.Decode(kb)
	if cp == nil || kp == nil {
		return nil, nil, errors.New("not PEM")
	}
	cert, err := x509.ParseCertificate(cp.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParseECPrivateKey(kp.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// IssueAgent adds a client certificate for name, signed by the CA that Init
// created in dir. It keeps a valid existing certificate (created is false).
func IssueAgent(dir, name string) (certPath, keyPath string, created bool, err error) {
	if !ValidAgentName(name) {
		return "", "", false, fmt.Errorf("agent name %q must be 1-64 chars of [A-Za-z0-9._-]", name)
	}
	l := layout{dir}
	caCert, caKey, err := readPair(filepath.Join(l.certs(), "ca.crt"), filepath.Join(l.certs(), "ca.key"))
	if err != nil {
		return "", "", false, fmt.Errorf("no quickstart CA in %s (start the quickstart first): %w", l.certs(), err)
	}
	created, err = ensureLeaf(l, name, name, nil, x509.ExtKeyUsageClientAuth, caCert, caKey, false)
	return filepath.Join(l.certs(), name+".crt"), filepath.Join(l.certs(), name+".key"), created, err
}
