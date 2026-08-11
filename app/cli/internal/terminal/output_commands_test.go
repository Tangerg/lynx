package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
)

func TestParseExportArgumentSeparatesTheFormatFromAnOptionalSpacedFilename(t *testing.T) {
	format, filename, err := parseExportArgument("md Project notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if format != sessiontransfer.Markdown || filename != "Project notes.md" {
		t.Fatalf("export argument = %q, %q", format, filename)
	}
	if _, _, err := parseExportArgument("pdf report.pdf"); err == nil {
		t.Fatal("unsupported export format was accepted")
	}
}

type copyTestHost struct {
	*programtest.Host
	copied chan string
}

func (h *copyTestHost) Copy(value string) bool {
	h.copied <- value
	return true
}

type outputTransferStub struct{}

func (outputTransferStub) ExportSession(_ context.Context, request sessiontransfer.ExportRequest) (sessiontransfer.Document, error) {
	if err := request.Validate(); err != nil {
		return sessiontransfer.Document{}, err
	}
	if request.Format == sessiontransfer.Markdown {
		return sessiontransfer.NewDocument(sessiontransfer.Markdown, []byte("# Runtime export\n\nstable answer\n"))
	}
	return sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(`{"version":17}`))
}

func (outputTransferStub) ImportSession(context.Context, sessiontransfer.ImportRequest) (agent.Session, error) {
	return agent.Session{}, errors.New("unexpected import")
}

func runUIWithCopyHost(t *testing.T, backend agent.Runtime, workspace string) (*copyTestHost, func()) {
	t.Helper()
	host := &copyTestHost{Host: programtest.New(t, 96, 28), copied: make(chan string, 8)}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: backend, Transfers: outputTransferStub{}, Workspace: workspace, Host: host})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestCopyLastAndExportCommandsUseTheDurableSessionSnapshot(t *testing.T) {
	workspace := t.TempDir()
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	host, stop := runUIWithCopyHost(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("produce a durable answer")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	host.Shows(t, "complete")

	host.Type("/copy-last")
	host.Press(input.Enter)
	select {
	case copied := <-host.copied:
		if strings.TrimSpace(copied) != "stable answer" {
			t.Fatalf("copied = %q", copied)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for clipboard copy")
	}

	host.Type("/export markdown report.md")
	host.Press(input.Enter)
	host.Shows(t, "exported session")
	path := filepath.Join(workspace, "report.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "stable answer") {
		t.Fatalf("export does not contain the durable assistant response:\n%s", data)
	}
	stop()
}
