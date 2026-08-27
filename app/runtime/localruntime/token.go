// Package localruntime owns deployment handoff material shared by a local
// Runtime process and its trusted application clients.
package localruntime

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	rawTokenBytes         = 32
	encodedTokenBytes     = 43
	tokenCandidatePattern = ".local-token-*"
)

// ErrInvalidToken identifies durable token material that cannot be trusted as
// the canonical credential published by OpenToken.
var ErrInvalidToken = errors.New("invalid local Runtime token")

// Token is a validated local Runtime credential and the durable path that owns
// its lifecycle. Its fields are private so callers cannot mistake unvalidated
// file contents for a credential.
type Token struct {
	value string
	path  string
}

// Value returns the canonical bearer value.
func (t *Token) Value() string {
	if t == nil {
		return ""
	}
	return t.value
}

// Path returns the absolute durable credential path.
func (t *Token) Path() string {
	if t == nil {
		return ""
	}
	return t.path
}

// OpenToken loads the credential owned by path, creating and atomically
// publishing one when it does not exist. Concurrent Runtime generations converge
// on the first published value rather than creating process-owned credentials.
func OpenToken(path string) (*Token, error) {
	path, err := cleanTokenPath(path)
	if err != nil {
		return nil, err
	}
	token, err := ReadToken(path)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	parent := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(parent, 0o700); mkdirErr != nil {
		return nil, fmt.Errorf("local Runtime token: create parent: %w", mkdirErr)
	}
	value, err := newTokenValue()
	if err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(parent, tokenCandidatePattern)
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: create candidate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: protect candidate: %w", err)
	}
	if _, err := temporary.WriteString(value); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: write candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("local Runtime token: sync candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("local Runtime token: close candidate: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ReadToken(path)
		}
		return nil, fmt.Errorf("local Runtime token: publish candidate: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return nil, fmt.Errorf("local Runtime token: sync parent: %w", err)
	}
	return &Token{value: value, path: path}, nil
}

// ReadToken validates and reads exactly one canonical 32-byte credential. It
// rejects links, replacements, non-private modes, padding, surrounding
// whitespace, and every file size other than the 43-byte raw URL encoding.
func ReadToken(path string) (*Token, error) {
	path, err := cleanTokenPath(path)
	if err != nil {
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if validationErr := validateTokenFile(pathInfo); validationErr != nil {
		return nil, validationErr
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: open: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("local Runtime token: inspect opened file: %w", err)
	}
	if !os.SameFile(pathInfo, openedInfo) {
		return nil, invalidToken("path changed while opening")
	}
	if validationErr := validateTokenFile(openedInfo); validationErr != nil {
		return nil, validationErr
	}

	var encoded [encodedTokenBytes + 1]byte
	n, err := io.ReadFull(file, encoded[:])
	if n != encodedTokenBytes || !errors.Is(err, io.ErrUnexpectedEOF) {
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("local Runtime token: read: %w", err)
		}
		return nil, invalidToken("file must contain exactly 43 bytes")
	}

	var decoded [rawTokenBytes]byte
	decodedBytes, err := base64.RawURLEncoding.Decode(decoded[:], encoded[:encodedTokenBytes])
	if err != nil || decodedBytes != rawTokenBytes ||
		base64.RawURLEncoding.EncodeToString(decoded[:]) != string(encoded[:encodedTokenBytes]) {
		return nil, invalidToken("file does not contain one canonical 32-byte token")
	}
	return &Token{value: string(encoded[:encodedTokenBytes]), path: path}, nil
}

func cleanTokenPath(path string) (string, error) {
	if path == "" {
		return "", invalidToken("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", invalidToken("path must be absolute")
	}
	return filepath.Clean(path), nil
}

func validateTokenFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return invalidToken("path must be a regular file")
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		return invalidToken(fmt.Sprintf("permissions are %04o, want 0600", permissions))
	}
	if info.Size() != encodedTokenBytes {
		return invalidToken("file must contain exactly 43 bytes")
	}
	return nil
}

func newTokenValue() (string, error) {
	var raw [rawTokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("local Runtime token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func invalidToken(reason string) error {
	return fmt.Errorf("local Runtime token: %s: %w", reason, ErrInvalidToken)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
