package runtimeembedded

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type sessionBindingStub struct {
	rollback func(context.Context, protocol.RollbackSessionRequest, embedded.CommandOptions) (*protocol.RollbackSessionResponse, error)
	export   func(context.Context, protocol.ExportSessionRequest, embedded.CallOptions) (*protocol.ExportSessionResponse, error)
	imported func(context.Context, protocol.ImportSessionRequest, embedded.CommandOptions) (*protocol.ImportSessionResponse, error)
}

func sessionControlProfile(features ...runtimeprofile.FeatureName) runtimeprofile.Profile {
	profile := runtimeprofile.Profile{Features: make(map[runtimeprofile.FeatureName]runtimeprofile.Feature, len(features))}
	for _, feature := range features {
		profile.Features[feature] = runtimeprofile.Feature{Enabled: true}
	}
	return profile
}

func (s sessionBindingStub) RollbackSession(ctx context.Context, request protocol.RollbackSessionRequest, options embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
	return s.rollback(ctx, request, options)
}

func (s sessionBindingStub) ExportSession(ctx context.Context, request protocol.ExportSessionRequest, options embedded.CallOptions) (*protocol.ExportSessionResponse, error) {
	return s.export(ctx, request, options)
}

func (s sessionBindingStub) ImportSession(ctx context.Context, request protocol.ImportSessionRequest, options embedded.CommandOptions) (*protocol.ImportSessionResponse, error) {
	return s.imported(ctx, request, options)
}

func TestSessionControlProjectsRollbackWithoutLosingInlineInput(t *testing.T) {
	image := []byte("image body")
	commandID := agent.CommandID("cli_77777777777777777777777777777777")
	stub := sessionBindingStub{}
	stub.rollback = func(_ context.Context, request protocol.RollbackSessionRequest, options embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
		if request.SessionID != "ses_1" || request.ToRunID != "run_1" || request.RestoreType != protocol.RestoreBoth {
			t.Fatalf("rollback request = %+v", request)
		}
		if options.IdempotencyKey != string(commandID) || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
			t.Fatalf("rollback options = %+v", options)
		}
		return &protocol.RollbackSessionResponse{
			Session: &protocol.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable)},
			DroppedRuns: []protocol.DroppedRun{{
				Run: protocol.RunSummary{ID: "run_2", SessionID: "ses_1", Status: protocol.RunStatusFinished},
				UserInput: []protocol.ContentBlock{
					{Type: protocol.ContentBlockText, Text: "try another approach"},
					{Type: protocol.ContentBlockImage, Mime: "image/png", Data: base64.StdEncoding.EncodeToString(image)},
				},
			}},
		}, nil
	}
	runtime := &Runtime{
		sessions: stub, meta: requestMeta("test"),
		profile: sessionControlProfile(runtimeprofile.FeatureCheckpoints),
	}
	result, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		CommandID: commandID, SessionID: "ses_1", ToRunID: "run_1", Scope: agent.RestoreBoth,
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

func TestSessionControlRejectsCrossSessionResponses(t *testing.T) {
	t.Parallel()
	stub := sessionBindingStub{
		rollback: func(context.Context, protocol.RollbackSessionRequest, embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
			return &protocol.RollbackSessionResponse{Session: &protocol.Session{
				ID: "ses_other", Status: protocol.SessionStatusIdle,
				Provider: testSessionProvider, Model: testSessionModel,
				Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
			}}, nil
		},
		export: func(context.Context, protocol.ExportSessionRequest, embedded.CallOptions) (*protocol.ExportSessionResponse, error) {
			return &protocol.ExportSessionResponse{Format: protocol.ExportFormatJSON, Artifact: &protocol.SessionArtifact{
				Version: protocol.SessionArtifactVersion,
				Session: protocol.ArtifactSession{ID: "ses_other", Workspace: protocol.WorkspaceRef{Path: "/workspace"}, Provider: testSessionProvider, Model: testSessionModel},
			}}, nil
		},
	}
	runtime := &Runtime{
		sessions: stub, meta: requestMeta("test"),
		profile: sessionControlProfile(runtimeprofile.FeatureSessionExport),
	}
	_, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{SessionID: "ses_1", Scope: agent.RestoreHistory})
	requireRuntimeContractViolation(t, err)
	_, err = runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{SessionID: "ses_1", Format: sessiontransfer.JSON})
	requireRuntimeContractViolation(t, err)
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
				Session: protocol.ArtifactSession{ID: "ses_1", Title: "Session", Workspace: protocol.WorkspaceRef{Path: "/workspace"}, Provider: testSessionProvider, Model: testSessionModel},
			}}, nil
		default:
			return nil, errors.New("unexpected format")
		}
	}
	runtime := &Runtime{
		sessions: stub, meta: requestMeta("test"),
		profile: sessionControlProfile(runtimeprofile.FeatureSessionExport),
	}
	markdown, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{SessionID: "ses_1", Format: sessiontransfer.Markdown})
	if err != nil || string(markdown.Bytes()) != "# Runtime transcript" {
		t.Fatalf("Markdown export = (%q, %v)", markdown.Bytes(), err)
	}
	artifact, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{SessionID: "ses_1", Format: sessiontransfer.JSON})
	versionField := fmt.Sprintf(`"version": %d`, protocol.SessionArtifactVersion)
	if err != nil || !strings.Contains(string(artifact.Bytes()), versionField) || !artifact.Importable() {
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
			ID: request.Artifact.Session.ID, Title: request.Artifact.Session.Title,
			Status: protocol.SessionStatusIdle, Provider: request.Artifact.Session.Provider, Model: request.Artifact.Session.Model,
			Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
			CreatedAt: request.Artifact.Session.CreatedAt, UpdatedAt: request.Artifact.Session.UpdatedAt.Add(time.Second),
			Favorite: request.Artifact.Session.Favorite,
		}}, nil
	}
	runtime := &Runtime{
		sessions: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable,
		}},
		meta:    requestMeta("test"),
		profile: sessionControlProfile(runtimeprofile.FeatureSessionExport),
	}
	artifactJSON := fmt.Sprintf(`{"version":%d,"session":{"id":"ses_1","title":"Session","workspace":{"path":"/workspace/alias"},"provider":"mock","model":"balanced","createdAt":"0001-01-01T00:00:00Z","updatedAt":"0001-01-01T00:00:00Z"},"messages":[],"runs":[],"items":[],"toolResults":[]}`, protocol.SessionArtifactVersion)
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(artifactJSON))
	if err != nil {
		t.Fatal(err)
	}
	session, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: document})
	if err != nil || session.ID != "ses_1" {
		t.Fatalf("ImportSession = (%+v, %v)", session, err)
	}

	unknownJSON := fmt.Sprintf(`{"version":%d,"future":true}`, protocol.SessionArtifactVersion)
	unknown, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(unknownJSON))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: unknown}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown artifact field error = %v", err)
	}
}

func TestSessionImportRejectsAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.August, 14, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{
			ID: "ses_1", Title: "Imported", Workspace: protocol.WorkspaceRef{Path: "/workspace"},
			Provider: "deepseek", Model: "deep", CreatedAt: createdAt, UpdatedAt: updatedAt, Favorite: true,
		},
	}
	body, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, body)
	if err != nil {
		t.Fatal(err)
	}
	resolvedWorkspace := workspace.Workspace{Path: "/workspace", ProjectRoot: "/workspace", Availability: workspace.Available}
	valid := protocol.Session{
		ID: artifact.Session.ID, Title: artifact.Session.Title, Status: protocol.SessionStatusIdle,
		Provider: artifact.Session.Provider, Model: artifact.Session.Model,
		Workspace: testProtocolWorkspace(artifact.Session.Workspace.Path, artifact.Session.Workspace.Path, protocol.WorkspaceAvailable),
		CreatedAt: artifact.Session.CreatedAt, UpdatedAt: artifact.Session.UpdatedAt.Add(time.Second),
		Favorite: artifact.Session.Favorite, Revision: 1,
	}
	tests := []struct {
		name   string
		mutate func(*protocol.Session)
	}{
		{name: "title", mutate: func(result *protocol.Session) { result.Title = "ignored" }},
		{name: "workspace", mutate: func(result *protocol.Session) { result.Workspace.Ref.Path = "/other" }},
		{name: "workspace project root", mutate: func(result *protocol.Session) { result.Workspace.ProjectRoot = "/other" }},
		{name: "workspace availability", mutate: func(result *protocol.Session) { result.Workspace.Availability = protocol.WorkspaceMissing }},
		{name: "model", mutate: func(result *protocol.Session) { result.Model = "shallow" }},
		{name: "favorite", mutate: func(result *protocol.Session) { result.Favorite = false }},
		{name: "created time", mutate: func(result *protocol.Session) { result.CreatedAt = result.CreatedAt.Add(time.Second) }},
		{name: "updated time moves backward", mutate: func(result *protocol.Session) { result.UpdatedAt = artifact.Session.UpdatedAt.Add(-time.Second) }},
		{name: "status", mutate: func(result *protocol.Session) { result.Status = protocol.SessionStatusRunning }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := valid
			test.mutate(&result)
			stub := sessionBindingStub{imported: func(context.Context, protocol.ImportSessionRequest, embedded.CommandOptions) (*protocol.ImportSessionResponse, error) {
				return &protocol.ImportSessionResponse{Session: &result}, nil
			}}
			runtime := &Runtime{
				sessions: stub,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref: protocol.WorkspaceRef{Path: resolvedWorkspace.Path}, ProjectRoot: resolvedWorkspace.ProjectRoot,
					Availability: protocol.WorkspaceAvailable,
				}},
				meta:    requestMeta("test"),
				profile: sessionControlProfile(runtimeprofile.FeatureSessionExport),
			}
			_, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: document})
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestSessionControlRejectsConditionalOperationsBeforeCallingBinding(t *testing.T) {
	t.Parallel()
	called := false
	stub := sessionBindingStub{
		rollback: func(context.Context, protocol.RollbackSessionRequest, embedded.CommandOptions) (*protocol.RollbackSessionResponse, error) {
			called = true
			return nil, nil
		},
		export: func(context.Context, protocol.ExportSessionRequest, embedded.CallOptions) (*protocol.ExportSessionResponse, error) {
			called = true
			return nil, nil
		},
		imported: func(context.Context, protocol.ImportSessionRequest, embedded.CommandOptions) (*protocol.ImportSessionResponse, error) {
			called = true
			return nil, nil
		},
	}
	runtime := &Runtime{sessions: stub, meta: requestMeta("test")}
	if _, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_1", ToRunID: "run_1", Scope: agent.RestoreFiles,
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("files rollback error = %v, want ErrIncompatibleRuntime", err)
	}
	if _, err := runtime.ExportSession(t.Context(), sessiontransfer.ExportRequest{
		SessionID: "ses_1", Format: sessiontransfer.JSON,
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("export error = %v, want ErrIncompatibleRuntime", err)
	}
	artifactJSON := fmt.Sprintf(`{"version":%d,"session":{"id":"ses_1","workspace":{"path":"/workspace"},"provider":"mock","model":"balanced"},"messages":[],"runs":[],"items":[],"toolResults":[]}`, protocol.SessionArtifactVersion)
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(artifactJSON))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ImportSession(t.Context(), sessiontransfer.ImportRequest{Artifact: document}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("import error = %v, want ErrIncompatibleRuntime", err)
	}
	if called {
		t.Fatal("conditional session control reached the binding without negotiated capabilities")
	}
}
