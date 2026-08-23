package localruntime

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxTokenBytes = 128

func OpenToken(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("local Runtime token: path must be absolute")
	}
	value, err := ReadToken(path)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("local Runtime token: create parent: %w", err)
	}
	if err := requirePrivateDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	value, err = randomValue(32)
	if err != nil {
		return "", fmt.Errorf("local Runtime token: create value: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".runtime-token-*")
	if err != nil {
		return "", fmt.Errorf("local Runtime token: create candidate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(value); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("local Runtime token: write candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("local Runtime token: sync candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("local Runtime token: close candidate: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ReadToken(path)
		}
		return "", fmt.Errorf("local Runtime token: publish candidate: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("local Runtime token: sync parent: %w", err)
	}
	return value, nil
}

func ReadToken(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("local Runtime token: path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return "", errors.New("local Runtime token: path must be a 0600 regular file")
	}
	if info.Size() > maxTokenBytes {
		return "", errors.New("local Runtime token: file is too large")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("local Runtime token: read: %w", err)
	}
	value := strings.TrimSpace(string(encoded))
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("local Runtime token: file does not contain one 32-byte token")
	}
	return value, nil
}

func randomValue(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
