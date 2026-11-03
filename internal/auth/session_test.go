package auth

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestSessionRoundTripAndExpiry(t *testing.T) {
	c, err := NewSessionCodec([]byte(strings.Repeat("k", 32)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	c.now = func() time.Time { return now }

	v, err := c.Issue("alice")
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Verify(v)
	if err != nil || s.User != "alice" {
		t.Fatalf("Verify = %+v, %v", s, err)
	}

	now = now.Add(time.Hour)
	if _, err := c.Verify(v); err == nil {
		t.Error("expired session accepted")
	}
}

func TestSessionRejectsForgery(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	c, _ := NewSessionCodec(key, time.Hour)
	v, _ := c.Issue("alice")

	other, _ := NewSessionCodec([]byte(strings.Repeat("x", 32)), time.Hour)
	forged, _ := other.Issue("admin")

	payload, sig, _ := strings.Cut(v, ".")
	for name, val := range map[string]string{
		"other key":     forged,
		"no signature":  payload,
		"bad signature": payload + "." + sig[:len(sig)-2] + "AA",
		"swap payload":  strings.Split(forged, ".")[0] + "." + sig,
		"empty":         "",
	} {
		if _, err := c.Verify(val); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSessionKeyTooShort(t *testing.T) {
	if _, err := NewSessionCodec([]byte("short"), time.Hour); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestParseUsers(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	// htpasswd -B emits $2y$; it must be accepted too.
	h2y := "$2y$" + string(hash)[4:]
	s, err := ParseUsers(strings.NewReader("# comment\n\nalice:" + string(hash) + "\nbob:" + h2y + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Authenticate("alice", "pw") || !s.Authenticate("bob", "pw") {
		t.Error("valid credentials rejected")
	}
	if s.Authenticate("alice", "nope") || s.Authenticate("carol", "pw") || s.Authenticate("", "") {
		t.Error("invalid credentials accepted")
	}

	for name, in := range map[string]string{
		"empty":      "# nothing\n",
		"no colon":   "alice\n",
		"plaintext":  "alice:hunter2\n",
		"md5 apr1":   "alice:$apr1$abc$def\n",
		"duplicate":  "a:" + string(hash) + "\na:" + string(hash) + "\n",
		"empty user": ":" + string(hash) + "\n",
	} {
		if _, err := ParseUsers(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
