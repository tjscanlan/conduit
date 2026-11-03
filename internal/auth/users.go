package auth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// dummyHash is compared against when a username is unknown, so a login
// attempt costs the same whether or not the user exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("conduit-dummy-password"), bcrypt.DefaultCost)

// UserStore holds UI login credentials as bcrypt hashes.
type UserStore struct {
	hashes map[string][]byte
}

// LoadUsersFile reads an htpasswd-style file of "username:bcrypt-hash" lines,
// as produced by `htpasswd -nB user`. Blank lines and # comments are ignored.
func LoadUsersFile(path string) (*UserStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseUsers(f)
}

// ParseUsers parses htpasswd-style bcrypt entries. Non-bcrypt hashes are rejected.
func ParseUsers(r io.Reader) (*UserStore, error) {
	s := &UserStore{hashes: make(map[string][]byte)}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		user, hash, ok := strings.Cut(line, ":")
		if !ok || user == "" {
			return nil, fmt.Errorf("users file line %d: expected username:hash", n)
		}
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return nil, fmt.Errorf("users file line %d: hash for %q is not bcrypt", n, user)
		}
		if _, dup := s.hashes[user]; dup {
			return nil, fmt.Errorf("users file line %d: duplicate user %q", n, user)
		}
		s.hashes[user] = []byte(hash)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(s.hashes) == 0 {
		return nil, errors.New("users file contains no users")
	}
	return s, nil
}

// Authenticate reports whether password is correct for user.
func (s *UserStore) Authenticate(user, password string) bool {
	hash, ok := s.hashes[user]
	if !ok {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}
