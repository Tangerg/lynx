package localruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const BootstrapVersion = 1

const maxDescriptorBytes = 64 << 10

var ErrInvalidDescriptor = errors.New("invalid local Runtime descriptor")

type Descriptor struct {
	BootstrapVersion int    `json:"bootstrapVersion"`
	Nonce            string `json:"nonce"`
	PID              int    `json:"pid"`
	InstanceID       string `json:"instanceId"`
	ProtocolVersion  string `json:"protocolVersion"`
	BaseURL          string `json:"baseUrl"`
	TokenPath        string `json:"tokenPath"`
}

type Expectation struct {
	Root            string
	Nonce           string
	PID             int
	ProtocolVersion string
}

func NewNonce() (string, error) {
	nonce, err := randomValue(24)
	if err != nil {
		return "", fmt.Errorf("local Runtime descriptor: create nonce: %w", err)
	}
	return nonce, nil
}

func Publish(path string, descriptor Descriptor) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("local Runtime descriptor: path must be absolute: %w", ErrInvalidDescriptor)
	}
	if err := validateDescriptor(descriptor); err != nil {
		return err
	}
	parent := filepath.Dir(filepath.Clean(path))
	if err := requirePrivateDirectory(parent); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(parent, ".runtime-ready-*")
	if err != nil {
		return fmt.Errorf("local Runtime descriptor: create candidate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(descriptor); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("local Runtime descriptor: encode candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("local Runtime descriptor: sync candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("local Runtime descriptor: close candidate: %w", err)
	}

	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("local Runtime descriptor: publish candidate: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return fmt.Errorf("local Runtime descriptor: sync parent: %w", err)
	}
	return nil
}

func Consume(path string, expected Expectation) (Descriptor, error) {
	if !filepath.IsAbs(path) || !filepath.IsAbs(expected.Root) || !containedBy(expected.Root, path) {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: path is outside the spawn root: %w", ErrInvalidDescriptor)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Descriptor{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: path is not a private regular file: %w", ErrInvalidDescriptor)
	}
	if info.Size() > maxDescriptorBytes {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: file exceeds %d bytes: %w", maxDescriptorBytes, ErrInvalidDescriptor)
	}
	file, err := os.Open(path)
	if err != nil {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: open: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, maxDescriptorBytes+1))
	decoder.DisallowUnknownFields()
	var descriptor Descriptor
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: decode: %w", errors.Join(ErrInvalidDescriptor, err))
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Descriptor{}, err
	}
	if err := validateDescriptor(descriptor); err != nil {
		return Descriptor{}, err
	}
	if descriptor.Nonce != expected.Nonce || descriptor.PID != expected.PID || descriptor.ProtocolVersion != expected.ProtocolVersion {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: generation identity mismatch: %w", ErrInvalidDescriptor)
	}
	if !containedBy(expected.Root, descriptor.TokenPath) {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: token path is outside the spawn root: %w", ErrInvalidDescriptor)
	}
	if err := os.Remove(path); err != nil {
		return Descriptor{}, fmt.Errorf("local Runtime descriptor: consume: %w", err)
	}
	return descriptor, nil
}

func validateDescriptor(descriptor Descriptor) error {
	switch {
	case descriptor.BootstrapVersion != BootstrapVersion:
		return fmt.Errorf("local Runtime descriptor: bootstrap version %d is unsupported: %w", descriptor.BootstrapVersion, ErrInvalidDescriptor)
	case descriptor.Nonce == "":
		return fmt.Errorf("local Runtime descriptor: nonce is required: %w", ErrInvalidDescriptor)
	case descriptor.PID <= 0:
		return fmt.Errorf("local Runtime descriptor: pid must be positive: %w", ErrInvalidDescriptor)
	case descriptor.InstanceID == "":
		return fmt.Errorf("local Runtime descriptor: instanceId is required: %w", ErrInvalidDescriptor)
	case descriptor.ProtocolVersion == "":
		return fmt.Errorf("local Runtime descriptor: protocolVersion is required: %w", ErrInvalidDescriptor)
	case !filepath.IsAbs(descriptor.TokenPath):
		return fmt.Errorf("local Runtime descriptor: tokenPath must be absolute: %w", ErrInvalidDescriptor)
	}
	endpoint, err := url.Parse(descriptor.BaseURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Host == "" ||
		(endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("local Runtime descriptor: baseUrl is invalid: %w", ErrInvalidDescriptor)
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return fmt.Errorf("local Runtime descriptor: baseUrl must use a loopback IP: %w", ErrInvalidDescriptor)
	}
	if endpoint.Port() == "" {
		return fmt.Errorf("local Runtime descriptor: baseUrl must include a port: %w", ErrInvalidDescriptor)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("local Runtime descriptor: trailing data: %w", errors.Join(ErrInvalidDescriptor, err))
	}
	return fmt.Errorf("local Runtime descriptor: contains multiple JSON values: %w", ErrInvalidDescriptor)
}

func containedBy(root, candidate string) bool {
	cleanRoot := filepath.Clean(root)
	cleanCandidate := filepath.Clean(candidate)
	if !filepath.IsAbs(cleanRoot) || !filepath.IsAbs(cleanCandidate) {
		return false
	}
	relative, err := filepath.Rel(cleanRoot, cleanCandidate)
	if err != nil || relative == "." {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("local Runtime descriptor: inspect parent: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("local Runtime descriptor: parent is not a private directory: %w", ErrInvalidDescriptor)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
