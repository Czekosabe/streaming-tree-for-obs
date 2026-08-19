package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/streaming-tree/server/internal/auth"
	"golang.org/x/term"
)

// readProvisioningPassword reads the new administrator password for
// --provision-admin-password (docs/remote-management.md §9.2/§14):
// never a command-line argument, never an environment variable.
//
// When stdin is a real terminal, it prompts twice with hidden input
// (golang.org/x/term.ReadPassword) and requires the two entries to
// match - an interactive operator gets the usual confirmation
// safeguard against a silent typo. When stdin is not a terminal (a
// script or a native CI test driving this command non-interactively),
// it reads exactly one line - no confirmation round trip is possible
// without a real terminal, and requiring one would make this mode
// impossible to test or script.
func readProvisioningPassword() (string, error) {
	stdinFD := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFD) {
		return readPasswordInteractive(stdinFD)
	}
	return readPasswordLine(os.Stdin)
}

func readPasswordInteractive(stdinFD int) (string, error) {
	fmt.Fprint(os.Stderr, "New administrator password: ")
	first, err := term.ReadPassword(stdinFD)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Confirm administrator password: ")
	second, err := term.ReadPassword(stdinFD)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading password confirmation: %w", err)
	}

	if string(first) != string(second) {
		return "", errors.New("the two entries did not match")
	}

	password := string(first)
	if len(password) == 0 {
		return "", errors.New("an empty password is not accepted")
	}
	if len(password) > auth.MaxPasswordLength {
		return "", fmt.Errorf("password must not exceed %d bytes", auth.MaxPasswordLength)
	}
	return password, nil
}

func readPasswordLine(r *os.File) (string, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("reading password from stdin: %w", err)
		}
		return "", errors.New("no password was provided on stdin")
	}
	password := strings.TrimRight(scanner.Text(), "\r\n")
	if len(password) == 0 {
		return "", errors.New("an empty password is not accepted")
	}
	if len(password) > auth.MaxPasswordLength {
		return "", fmt.Errorf("password must not exceed %d bytes", auth.MaxPasswordLength)
	}
	return password, nil
}
