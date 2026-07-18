package lsp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeModule lays down a tiny buildable Go module and returns its root. The
// LSP operations are exercised against real gopls, so the fixture must be a
// genuine module (gopls keys everything off go.mod).
func writeModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("go.mod", "module example.com/lsptest\n\ngo 1.21\n")
	// Line/char references below are 0-based, matching the wire.
	write("main.go", "package main\n"+ // 0
		"\n"+ // 1
		"func Greet(name string) string {\n"+ // 2: decl of Greet at char 5
		"\treturn \"hi \" + name\n"+ // 3
		"}\n"+ // 4
		"\n"+ // 5
		"func main() {\n"+ // 6
		"\t_ = Greet(\"world\")\n"+ // 7: use of Greet at char 5
		"}\n") // 8
	write("bad.go", "package main\n\nfunc bad() {\n\tundefinedSymbol()\n}\n")
	return root
}

// gopls is at minimum 3s of cold start; give the whole suite generous slack.
func newTestServers(t *testing.T) *Servers {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed; skipping LSP integration test")
	}
	servers := NewServers(DefaultServers())
	t.Cleanup(func() { _ = servers.Close() })
	return servers
}

func TestServers_DefinitionAndHover(t *testing.T) {
	servers := newTestServers(t)
	root := writeModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Definition of Greet at its use site (main.go:7:5) → its declaration (line 2).
	locs, err := servers.Definition(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("Definition returned no locations")
	}
	if got := locs[0].Range.Start.Line; got != 2 {
		t.Errorf("Definition start line = %d, want 2", got)
	}
	if !strings.HasSuffix(uriToPath(locs[0].URI), "main.go") {
		t.Errorf("Definition uri = %q, want .../main.go", locs[0].URI)
	}

	// Hover at the same spot mentions the function.
	hov, err := servers.Hover(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if !strings.Contains(hov, "Greet") {
		t.Errorf("Hover = %q, want it to mention Greet", hov)
	}
}

func TestServers_ReferencesAndSymbols(t *testing.T) {
	servers := newTestServers(t)
	root := writeModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// References to Greet (declaration at main.go:2:5) → declaration + the call.
	refs, err := servers.References(ctx, root, "main.go", Position{Line: 2, Character: 5})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(refs) < 2 {
		t.Errorf("References = %d, want >= 2 (declaration + use)", len(refs))
	}

	// Document symbols include both functions.
	syms, err := servers.DocumentSymbols(ctx, root, "main.go")
	if err != nil {
		t.Fatalf("DocumentSymbols: %v", err)
	}
	if !hasSymbol(syms, "Greet") || !hasSymbol(syms, "main") {
		t.Errorf("DocumentSymbols = %v, want Greet and main", names(syms))
	}

	// Workspace symbol search finds Greet across the module.
	ws, err := servers.WorkspaceSymbols(ctx, root, "Greet")
	if err != nil {
		t.Fatalf("WorkspaceSymbols: %v", err)
	}
	if !hasSymbol(ws, "Greet") {
		t.Errorf("WorkspaceSymbols(Greet) = %v, want Greet", names(ws))
	}
}

func TestServers_Diagnostics(t *testing.T) {
	servers := newTestServers(t)
	root := writeModule(t)

	// Cold gopls can take longer than one settle window to first analyze; poll.
	var diags []Diagnostic
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		d, err := servers.Diagnostics(ctx, root, "bad.go")
		cancel()
		if err != nil {
			t.Fatalf("Diagnostics: %v", err)
		}
		if len(d) > 0 {
			diags = d
			break
		}
	}
	if len(diags) == 0 {
		t.Fatal("Diagnostics returned none for a file with an undefined symbol")
	}
	if diags[0].SeverityName() != "error" {
		t.Errorf("diagnostic severity = %q, want error", diags[0].SeverityName())
	}
}

func TestServers_UnsupportedFile(t *testing.T) {
	servers := NewServers(DefaultServers())
	t.Cleanup(func() { _ = servers.Close() })
	if servers.Supported("notes.txt") {
		t.Error("Supported(.txt) = true, want false")
	}
	if servers.Supported("main.go") != true {
		t.Error("Supported(.go) = false, want true")
	}
	_, err := servers.Definition(context.Background(), t.TempDir(), "notes.txt", Position{})
	if err == nil {
		t.Fatal("Definition on unsupported file should error")
	}
}

func TestNewServersSnapshotsSpecs(t *testing.T) {
	specs := []ServerSpec{{
		Name:        "test",
		Command:     "test-lsp",
		Args:        []string{"before"},
		Extensions:  []string{".before"},
		RootMarkers: []string{"before.mod"},
	}}
	servers := NewServers(specs)
	t.Cleanup(func() { _ = servers.Close() })
	specs[0].Args[0] = "after"
	specs[0].Extensions[0] = ".after"
	specs[0].RootMarkers[0] = "after.mod"

	if !servers.Supported("file.before") || servers.Supported("file.after") {
		t.Fatal("server table retained caller-owned spec storage")
	}
}

func TestServersShareConcurrentStartup(t *testing.T) {
	servers := NewServers(nil)
	t.Cleanup(func() { _ = servers.Close() })
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	want := inertClient(nil)
	servers.launch = func(ctx context.Context, _ ServerSpec, _ string) (*client, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	const callers = 8
	results := make(chan *client, callers)
	errs := make(chan error, callers)
	root := t.TempDir()
	var ready sync.WaitGroup
	ready.Add(callers)
	begin := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-begin
			got, err := servers.clientFor(context.Background(), root, ServerSpec{Name: "test"})
			results <- got
			errs <- err
		}()
	}
	ready.Wait()
	close(begin)
	<-started
	close(release)

	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("clientFor: %v", err)
		}
		if got := <-results; got != want {
			t.Fatalf("clientFor returned %p, want shared client %p", got, want)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("start calls = %d, want 1", got)
	}
}

func TestServersCloseCancelsAndJoinsStartupCleanup(t *testing.T) {
	servers := NewServers(nil)
	started := make(chan struct{})
	cleanupErr := errors.New("close late client")
	servers.launch = func(ctx context.Context, _ ServerSpec, _ string) (*client, error) {
		close(started)
		<-ctx.Done()
		// Simulate a handshake that completed just after shutdown cancellation.
		return inertClient(cleanupErr), nil
	}

	clientErr := make(chan error, 1)
	root := t.TempDir()
	go func() {
		_, err := servers.clientFor(context.Background(), root, ServerSpec{Name: "test"})
		clientErr <- err
	}()
	<-started

	if err := servers.Close(); !errors.Is(err, cleanupErr) {
		t.Fatalf("Close error = %v, want late cleanup error", err)
	}
	if err := <-clientErr; !errors.Is(err, ErrClosed) {
		t.Fatalf("clientFor error = %v, want ErrClosed", err)
	}
}

func TestServersCallerCancellationDoesNotCancelSharedStartup(t *testing.T) {
	servers := NewServers(nil)
	t.Cleanup(func() { _ = servers.Close() })
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	want := inertClient(nil)
	servers.launch = func(ctx context.Context, _ ServerSpec, _ string) (*client, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	root := t.TempDir()
	spec := ServerSpec{Name: "test"}
	go func() {
		_, err := servers.clientFor(ctx, root, spec)
		first <- err
	}()
	<-started
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first clientFor error = %v, want context.Canceled", err)
	}

	second := make(chan *client, 1)
	secondErr := make(chan error, 1)
	go func() {
		got, err := servers.clientFor(context.Background(), root, spec)
		second <- got
		secondErr <- err
	}()
	close(release)
	if err := <-secondErr; err != nil {
		t.Fatalf("second clientFor: %v", err)
	}
	if got := <-second; got != want {
		t.Fatalf("second clientFor returned %p, want shared client %p", got, want)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("start calls = %d, want 1", got)
	}
}

func inertClient(closeErr error) *client {
	c := &client{closeErr: closeErr}
	c.closeOnce.Do(func() {})
	return c
}

func hasSymbol(syms []Symbol, name string) bool {
	for _, s := range syms {
		if s.Name == name {
			return true
		}
	}
	return false
}

func names(syms []Symbol) []string {
	var out []string
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}
