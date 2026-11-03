package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const passwordHashCost = 12

// hashPassword implements `ui hash-password`: it reads a password from the
// first line of stdin and prints a bcrypt hash for a UI_USERS_FILE entry:
//
//	printf '%s' "$PASSWORD" | ui hash-password   # -> $2a$12$...
//
// It ships in the binary because the runtime image has no shell or htpasswd.
func hashPassword(stdin io.Reader, stdout, stderr io.Writer) int {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(stderr, "read password:", err)
		return 1
	}
	pw := strings.TrimRight(line, "\r\n")
	if len(pw) < 12 {
		fmt.Fprintln(stderr, "password must be at least 12 characters")
		return 1
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), passwordHashCost)
	if err != nil {
		fmt.Fprintln(stderr, "hash password:", err) // e.g. longer than 72 bytes
		return 1
	}
	fmt.Fprintln(stdout, string(hash))
	return 0
}
