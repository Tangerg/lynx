package runtimeembedded

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
)

type sessionBinding interface {
	RollbackSession(context.Context, protocol.RollbackSessionRequest, embedded.CommandOptions) (*protocol.RollbackSessionResponse, error)
	ExportSession(context.Context, protocol.ExportSessionRequest, embedded.CallOptions) (*protocol.ExportSessionResponse, error)
	ImportSession(context.Context, protocol.ImportSessionRequest, embedded.CommandOptions) (*protocol.ImportSessionResponse, error)
}

var _ sessiontransfer.Service = (*Runtime)(nil)

func (r *Runtime) RollbackSession(ctx context.Context, input agent.RollbackSession) (agent.RollbackResult, error) {
	if err := input.Validate(); err != nil {
		return agent.RollbackResult{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.RollbackResult{}, err
	}
	response, err := r.sessions.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: input.SessionID, ToRunID: input.ToRunID, RestoreType: protocol.RestoreType(input.Scope),
	}, options)
	if err != nil {
		return agent.RollbackResult{}, classifyError(err)
	}
	if response == nil || response.Session == nil {
		return agent.RollbackResult{}, runtimeContractViolation("rollback session returned an incomplete result")
	}
	result := agent.RollbackResult{Dropped: make([]agent.DroppedRun, 0, len(response.DroppedRuns))}
	result.Session, err = projectSession(*response.Session)
	if err != nil {
		return agent.RollbackResult{}, runtimeContractViolation("rollback session returned an invalid session: %v", err)
	}
	if result.Session.ID != input.SessionID {
		return agent.RollbackResult{}, runtimeContractViolation("rollback session returned session %q for %q", result.Session.ID, input.SessionID)
	}
	for _, dropped := range response.DroppedRuns {
		if dropped.Run.SessionID != input.SessionID {
			return agent.RollbackResult{}, runtimeContractViolation(
				"rollback session %q returned dropped run %q from %q",
				input.SessionID, dropped.Run.ID, dropped.Run.SessionID,
			)
		}
		projected, err := projectDroppedRun(dropped)
		if err != nil {
			return agent.RollbackResult{}, runtimeContractViolation("rollback session returned an invalid dropped run: %v", err)
		}
		result.Dropped = append(result.Dropped, projected)
	}
	if err := result.Validate(); err != nil {
		return agent.RollbackResult{}, runtimeContractViolation("rollback session returned an invalid projection: %v", err)
	}
	return result, nil
}

func projectDroppedRun(value protocol.DroppedRun) (agent.DroppedRun, error) {
	projected := agent.DroppedRun{RunID: value.Run.ID, Input: make([]agent.InputContent, 0, len(value.UserInput))}
	for index, content := range value.UserInput {
		switch content.Type {
		case protocol.ContentBlockText:
			projected.Input = append(projected.Input, agent.InputContent{Kind: agent.InputText, Text: content.Text})
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(content.Data)
			if err != nil {
				return agent.DroppedRun{}, fmt.Errorf("rollback dropped run %s image %d: %w", value.Run.ID, index+1, err)
			}
			projected.Input = append(projected.Input, agent.InputContent{Kind: agent.InputImage, MimeType: content.Mime, Data: data})
		default:
			return agent.DroppedRun{}, fmt.Errorf("rollback dropped run %s content %d has unsupported type %q", value.Run.ID, index+1, content.Type)
		}
	}
	if err := projected.Validate(); err != nil {
		return agent.DroppedRun{}, err
	}
	return projected, nil
}

func (r *Runtime) ExportSession(ctx context.Context, request sessiontransfer.ExportRequest) (sessiontransfer.Document, error) {
	if err := request.Validate(); err != nil {
		return sessiontransfer.Document{}, err
	}
	response, err := r.sessions.ExportSession(ctx, protocol.ExportSessionRequest{
		SessionID: request.SessionID, Format: protocol.ExportFormat(request.Format),
	}, r.callOptions())
	if err != nil {
		return sessiontransfer.Document{}, classifyError(err)
	}
	if response == nil {
		return sessiontransfer.Document{}, runtimeContractViolation("export session returned nil")
	}
	if protocol.ExportFormat(request.Format) != response.Format {
		return sessiontransfer.Document{}, runtimeContractViolation("export session returned format %q, want %q", response.Format, request.Format)
	}
	var body []byte
	switch request.Format {
	case sessiontransfer.Markdown:
		if response.Artifact != nil || response.Markdown == "" {
			return sessiontransfer.Document{}, runtimeContractViolation("export session returned a malformed Markdown result")
		}
		body = []byte(response.Markdown)
	case sessiontransfer.JSON:
		if response.Artifact == nil || response.Markdown != "" {
			return sessiontransfer.Document{}, runtimeContractViolation("export session returned a malformed JSON result")
		}
		if err := protocol.ValidateWireTree(*response.Artifact); err != nil {
			return sessiontransfer.Document{}, runtimeContractViolation("export session returned an invalid artifact: %v", err)
		}
		if response.Artifact.Session.ID != request.SessionID {
			return sessiontransfer.Document{}, runtimeContractViolation(
				"export session returned artifact for %q, want %q",
				response.Artifact.Session.ID, request.SessionID,
			)
		}
		body, err = json.MarshalIndent(response.Artifact, "", "  ")
		if err != nil {
			return sessiontransfer.Document{}, runtimeContractViolation("export session artifact cannot be encoded: %v", err)
		}
	}
	document, err := sessiontransfer.NewDocument(request.Format, body)
	if err != nil {
		return sessiontransfer.Document{}, runtimeContractViolation("export session returned an invalid document: %v", err)
	}
	return document, nil
}

func (r *Runtime) ImportSession(ctx context.Context, request sessiontransfer.ImportRequest) (agent.Session, error) {
	if err := request.Validate(); err != nil {
		return agent.Session{}, err
	}
	var artifact protocol.SessionArtifact
	decoder := json.NewDecoder(bytes.NewReader(request.Artifact.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return agent.Session{}, fmt.Errorf("import session: decode artifact: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return agent.Session{}, fmt.Errorf("import session: %w", err)
	}
	if err := protocol.ValidateWireTree(artifact); err != nil {
		return agent.Session{}, fmt.Errorf("import session: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	response, err := r.sessions.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact}, options)
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if response == nil || response.Session == nil {
		return agent.Session{}, runtimeContractViolation("import session returned an incomplete result")
	}
	return projectSessionResult("import session", artifact.Session.ID, response.Session, nil)
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode artifact trailer: %w", err)
	}
	return errors.New("artifact contains more than one JSON value")
}
