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
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
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

type blockingOutputTransfer struct {
	sessiontransfer.Service
	started  chan sessiontransfer.ExportRequest
	release  chan struct{}
	canceled chan struct{}
}

func (service *blockingOutputTransfer) ExportSession(
	ctx context.Context,
	request sessiontransfer.ExportRequest,
) (sessiontransfer.Document, error) {
	service.started <- request
	select {
	case <-service.release:
		return service.Service.ExportSession(ctx, request)
	case <-ctx.Done():
		close(service.canceled)
		return sessiontransfer.Document{}, context.Cause(ctx)
	}
}

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

func TestSessionExportOutlivesSameSessionProjectionReplacement(t *testing.T) {
	workspace := t.TempDir()
	backend := mock.New()
	session, err := backend.CreateSession(t.Context(), agent.CreateSession{
		Title: "Export ownership", Workspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	backend.Instant = true
	backend.Script = stableCompletedScript
	opened, err := backend.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID, Message: agent.Message{Text: "create export history"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
	transfer := &blockingOutputTransfer{
		Service: outputTransferStub{}, started: make(chan sessiontransfer.ExportRequest, 1),
		release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(transfer.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: session.ID, Transfers: transfer, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/export markdown owned.md")
	host.Press(input.Enter)
	request := awaitValue(t, transfer.started, "session export")
	if request.SessionID != session.ID {
		t.Fatalf("export session = %q, want %q", request.SessionID, session.ID)
	}
	title := "Projection changed during export"
	installChangedSessionProjection(t, backend, source, session.ID, title)
	host.Shows(t, "session refreshed after runtime change")
	select {
	case <-transfer.canceled:
		t.Fatal("session projection replacement canceled the export")
	default:
	}
	release()
	exported := filepath.Join(workspace, "owned.md")
	host.Until(t, "the session export artifact", func() bool {
		_, statErr := os.Stat(exported)
		return statErr == nil && host.Repaint()
	})
	stop()
}
