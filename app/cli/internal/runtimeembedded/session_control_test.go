package runtimeembedded

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
)

type sessionBindingStub struct {
	rollback func(context.Context, protocol.RollbackSessionRequest, embedded.CommandOptions) (*protocol.RollbackSessionResponse, error)
	export   func(context.Context, protocol.ExportSessionRequest, embedded.CallOptions) (*protocol.ExportSessionResponse, error)
	imported func(context.Context, protocol.ImportSessionRequest, embedded.CommandOptions) (*protocol.ImportSessionResponse, error)
}

func (stub sessionBindingStub) RollbackSession(ctx context.Context, request protocol.RollbackSessionRequest, options embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
	return stub.rollback(ctx, request, options)
}

func (stub sessionBindingStub) ExportSession(ctx context.Context, request protocol.ExportSessionRequest, options embedded.CallOptions) (*protocol.ExportSessionResponse, error) {
	return stub.export(ctx, request, options)
}

func (stub sessionBindingStub) ImportSession(ctx context.Context, request protocol.ImportSessionRequest, options embedded.CommandOptions) (*protocol.ImportSessionResponse, error) {
	return stub.imported(ctx, request, options)
}

func TestSessionControlProjectsRollbackWithoutLosingInlineInput(t *testing.T) {
	image := []byte("image body")
	stub := sessionBindingStub{}
	stub.rollback = func(_ context.Context, request protocol.RollbackSessionRequest, options embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
		if request.SessionID != "ses_1" || request.ToRunID != "run_1" || request.RestoreType != protocol.RestoreBoth {
			t.Fatalf("rollback request = %+v", request)
		}
		if options.IdempotencyKey == "" || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
			t.Fatalf("rollback options = %+v", options)
		}
		return &protocol.RollbackSessionResponse{
			Session: &protocol.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}}},
			DroppedRuns: []protocol.DroppedRun{{
				Run: protocol.RunSummary{ID: "run_2", SessionID: "ses_1", Status: protocol.RunStatusFinished},
				UserInput: []protocol.ContentBlock{
					{Type: protocol.ContentBlockText, Text: "try another approach"},
					{Type: protocol.ContentBlockImage, Mime: "image/png", Data: base64.StdEncoding.EncodeToString(image)},
				},
			}},
		}, nil
	}
	runtime := &Runtime{sessions: stub, meta: requestMeta("test")}
	result, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_1", ToRunID: "run_1", Scope: agent.RestoreBoth,
	})
	if err != nil {
		t.Fatal(err)
	}
	input, ok := result.FirstOpeningInput()
	if !ok || len(input.Input) != 2 || string(input.Input[1].Data) != string(image) {
		t.Fatalf("rollback result = %+v", result)
	}
	text, images := input.OpeningText()
	if text != "try another approach" || images != 1 {
		t.Fatalf("opening input = %q, %d", text, images)
	}
}

func TestSessionTransferPreservesRuntimeNativeFormats(t *testing.T) {
	stub := sessionBindingStub{}
	stub.export = func(_ context.Context, request protocol.ExportSessionRequest, _ embedded.CallOptions) (*protocol.ExportSessionResponse, error) {
		switch request.Format {
		case protocol.ExportFormatMarkdown:
			return &protocol.ExportSessionResponse{Format: request.Format, Markdown: "# Runtime transcript"}, nil
		case protocol.ExportFormatJSON:
			return &protocol.ExportSessionResponse{Format: request.Format, Artifact: &protocol.SessionArtifact{
				Version: protocol.SessionArtifactVersion,
				Session: protocol.ArtifactSession{ID: "ses_1", Title: "Session", Workspace: protocol.WorkspaceRef{Path: "/workspace"}},
			}}, nil
		default:
			return nil, errors.New("unexpected format")
		}
	}
	runtime := &Runtime{sessions: stub, meta: requestMeta("test")}
	markdown, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{SessionID: "ses_1", Format: sessiontransfer.Markdown})
	if err != nil || string(markdown.Bytes()) != "# Runtime transcript" {
		t.Fatalf("Markdown export = (%q, %v)", markdown.Bytes(), err)
	}
	artifact, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{SessionID: "ses_1", Format: sessiontransfer.JSON})
	if err != nil || !strings.Contains(string(artifact.Bytes()), `"version": 17`) || !artifact.Importable() {
		t.Fatalf("JSON export = (%q, %v)", artifact.Bytes(), err)
	}
}

func TestSessionImportDecodesOpaqueDocumentOnlyAtTheAdapterBoundary(t *testing.T) {
	stub := sessionBindingStub{}
	stub.imported = func(_ context.Context, request protocol.ImportSessionRequest, options embedded.CommandOptions) (*protocol.ImportSessionResponse, error) {
		if request.Artifact.Version != protocol.SessionArtifactVersion || options.IdempotencyKey == "" {
			t.Fatalf("import request = %+v, options = %+v", request, options)
		}
		return &protocol.ImportSessionResponse{Session: &protocol.Session{
			ID: "ses_imported", Status: protocol.SessionStatusIdle,
			Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}},
		}}, nil
	}
	runtime := &Runtime{sessions: stub, meta: requestMeta("test")}
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(`{"version":17,"session":{"id":"ses_1","title":"Session","workspace":{"path":"/workspace"},"model":"","createdAt":"0001-01-01T00:00:00Z","updatedAt":"0001-01-01T00:00:00Z"},"messages":[],"runs":[],"items":[],"toolResults":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	session, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: document})
	if err != nil || session.ID != "ses_imported" {
		t.Fatalf("ImportSession = (%+v, %v)", session, err)
	}

	unknown, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(`{"version":17,"future":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: unknown}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown artifact field error = %v", err)
	}
}
