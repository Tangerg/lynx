package localruntime_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/localruntime"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestDescriptorAndTokenAreOneShotPrivateHandoff(t *testing.T) {
	t.Parallel()

	root := privateTempDir(t)
	tokenPath := filepath.Join(root, "token")
	token, err := localruntime.OpenToken(tokenPath)
	if err != nil {
		t.Fatalf("OpenToken() error = %v", err)
	}
	descriptorPath := filepath.Join(root, "ready.json")
	descriptor := localruntime.Descriptor{
		BootstrapVersion: localruntime.BootstrapVersion,
		Nonce:            "nonce_test",
		PID:              os.Getpid(),
		InstanceID:       "ins_test",
		ProtocolVersion:  protocol.ProtocolVersion,
		BaseURL:          "http://127.0.0.1:32123",
		TokenPath:        tokenPath,
	}
	if err := localruntime.Publish(descriptorPath, descriptor); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	info, err := os.Lstat(descriptorPath)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("descriptor mode = %04o", info.Mode().Perm())
	}

	got, err := localruntime.Consume(descriptorPath, localruntime.Expectation{
		Root:            root,
		Nonce:           descriptor.Nonce,
		PID:             descriptor.PID,
		ProtocolVersion: protocol.ProtocolVersion,
	})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if got != descriptor {
		t.Fatalf("descriptor = %+v, want %+v", got, descriptor)
	}
	if _, err := os.Stat(descriptorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed descriptor still exists: %v", err)
	}
	readToken, err := localruntime.ReadToken(tokenPath)
	if err != nil {
		t.Fatalf("ReadToken() error = %v", err)
	}
	if readToken != token {
		t.Fatal("token changed during handoff")
	}
}

func TestPublishNeverOverwritesExistingDescriptor(t *testing.T) {
	t.Parallel()

	root := privateTempDir(t)
	path := filepath.Join(root, "ready.json")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err := localruntime.Publish(path, localruntime.Descriptor{
		BootstrapVersion: localruntime.BootstrapVersion,
		Nonce:            "nonce_test",
		PID:              os.Getpid(),
		InstanceID:       "ins_test",
		ProtocolVersion:  protocol.ProtocolVersion,
		BaseURL:          "http://127.0.0.1:32123",
		TokenPath:        filepath.Join(root, "token"),
	})
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("Publish() error = %v, want os.ErrExist", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "owned" {
		t.Fatalf("existing descriptor changed: data=%q error=%v", data, readErr)
	}
}

func TestConsumeRejectsEscapedTokenPathAndPartialJSON(t *testing.T) {
	t.Parallel()

	root := privateTempDir(t)
	path := filepath.Join(root, "ready.json")
	if err := os.WriteFile(path, []byte(`{"bootstrapVersion":1`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := localruntime.Consume(path, localruntime.Expectation{Root: root}); err == nil {
		t.Fatal("Consume() accepted partial JSON")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	escaped := localruntime.Descriptor{
		BootstrapVersion: localruntime.BootstrapVersion,
		Nonce:            "nonce_test",
		PID:              os.Getpid(),
		InstanceID:       "ins_test",
		ProtocolVersion:  protocol.ProtocolVersion,
		BaseURL:          "http://127.0.0.1:32123",
		TokenPath:        filepath.Join(filepath.Dir(root), "foreign-token"),
	}
	if err := localruntime.Publish(path, escaped); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if _, err := localruntime.Consume(path, localruntime.Expectation{
		Root: root, Nonce: escaped.Nonce, PID: escaped.PID, ProtocolVersion: protocol.ProtocolVersion,
	}); err == nil {
		t.Fatal("Consume() accepted a token path outside the spawn root")
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("Chmod(%q) error = %v", root, err)
	}
	return root
}
