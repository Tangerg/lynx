package runtimehost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/localruntime"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
)

func TestRuntimePublishesUsableIdentityAndReleasesGeneration(t *testing.T) {
	dataRoot := privateDirectory(t, "data")
	spawnRoot := privateDirectory(t, "spawn")
	descriptorPath := filepath.Join(spawnRoot, "ready.json")
	tokenPath := filepath.Join(spawnRoot, "token")
	nonce, err := localruntime.NewNonce()
	if err != nil {
		t.Fatalf("NewNonce() error = %v", err)
	}
	host, err := runtimehost.Open(t.Context(), runtimehost.Config{
		Listen:           "127.0.0.1:0",
		DatabasePath:     filepath.Join(dataRoot, "runtime.sqlite"),
		TokenPath:        tokenPath,
		DescriptorPath:   descriptorPath,
		BootstrapNonce:   nonce,
		DefaultWorkspace: "/workspace",
		UserHome:         "/home/test",
		ServerName:       "lyra-runtime",
		ServerVersion:    "test",
		CORSOrigins:      httptransport.DefaultCORSOrigins(),
	})
	if err != nil {
		t.Fatalf("runtimehost.Open() error = %v", err)
	}
	runCtx, cancel := context.WithCancel(t.Context())
	runResult := make(chan error, 1)
	go func() { runResult <- host.Run(runCtx) }()

	descriptor := awaitDescriptor(t, descriptorPath, localruntime.Expectation{
		Root: spawnRoot, Nonce: nonce, PID: os.Getpid(), ProtocolVersion: protocol.ProtocolVersion,
	})
	token, err := localruntime.ReadToken(descriptor.TokenPath)
	if err != nil {
		t.Fatalf("ReadToken() error = %v", err)
	}
	info := getJSON(t, descriptor.BaseURL+httptransport.PathInfo, "", nil)
	ready := getJSON(t, descriptor.BaseURL+httptransport.PathReadiness, "", nil)
	discovery := getJSON(t, descriptor.BaseURL+httptransport.PathRPC, token, []byte(
		`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`,
	))
	for name, document := range map[string]map[string]any{"info": info, "ready": ready} {
		if instanceID(document) != descriptor.InstanceID {
			t.Fatalf("%s instanceId = %q, want %q", name, instanceID(document), descriptor.InstanceID)
		}
	}
	result := discovery["result"].(map[string]any)
	serverInfo := result["serverInfo"].(map[string]any)
	if serverInfo["instanceId"] != descriptor.InstanceID {
		t.Fatalf("discover instanceId = %v, want %q", serverInfo["instanceId"], descriptor.InstanceID)
	}

	cancel()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not stop")
	}
	if connection, err := net.DialTimeout("tcp", descriptorAddress(t, descriptor.BaseURL), 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("Runtime listener survived shutdown")
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ephemeral token remains after shutdown: %v", err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestCorruptStoreNeverPublishesReadyDescriptor(t *testing.T) {
	dataRoot := privateDirectory(t, "data")
	spawnRoot := privateDirectory(t, "spawn")
	databasePath := filepath.Join(dataRoot, "runtime.sqlite")
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	descriptorPath := filepath.Join(spawnRoot, "ready.json")
	_, err := runtimehost.Open(t.Context(), runtimehost.Config{
		Listen: "127.0.0.1:0", DatabasePath: databasePath,
		TokenPath: filepath.Join(spawnRoot, "token"), DescriptorPath: descriptorPath, BootstrapNonce: "nonce",
		DefaultWorkspace: "/workspace", UserHome: "/home/test", ServerName: "lyra-runtime", ServerVersion: "test",
	})
	if err == nil {
		t.Fatal("Open() accepted a corrupt store")
	}
	if _, statErr := os.Stat(descriptorPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("descriptor was published after failed open: %v", statErr)
	}
}

func privateDirectory(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", path, err)
	}
	return path
}

func awaitDescriptor(t *testing.T, path string, expectation localruntime.Expectation) localruntime.Descriptor {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		descriptor, err := localruntime.Consume(path, expectation)
		if err == nil {
			return descriptor
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Consume() error = %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("descriptor was not published")
		}
	}
}

func getJSON(t *testing.T, endpoint, token string, body []byte) map[string]any {
	t.Helper()
	method := http.MethodGet
	var reader *bytes.Reader
	if body != nil {
		method = http.MethodPost
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status = %d", method, endpoint, response.StatusCode)
	}
	var document map[string]any
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatalf("decode %s error = %v", endpoint, err)
	}
	return document
}

func instanceID(document map[string]any) string {
	if server, ok := document["server"].(map[string]any); ok {
		return server["instanceId"].(string)
	}
	return document["instanceId"].(string)
}

func descriptorAddress(t *testing.T, baseURL string) string {
	t.Helper()
	return strings.TrimPrefix(baseURL, "http://")
}
