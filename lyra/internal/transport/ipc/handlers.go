package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	aguiencoder "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/encoder"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// ------------------------------------------------------------------
// agent.run — streaming
// ------------------------------------------------------------------

type agentRunParams struct {
	ThreadID string `json:"threadId,omitempty"`
	Message  string `json:"message"`
	PlanMode bool   `json:"planMode,omitempty"`
}

// handleAgentRun is the streaming entry point. It mirrors the HTTP
// transport's /v1/agent/run flow: resolve or auto-create a session,
// start a turn, drain the per-turn event channel through the AG-UI
// translator, and write each resulting event as one frame.
func (s *Server) handleAgentRun(ctx context.Context, req request) {
	var params agentRunParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.Message == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "message is required")
		return
	}

	sessionID, err := s.resolveSession(ctx, params.ThreadID)
	if err != nil {
		s.writeError(req.ID, "BAD_SESSION", err.Error())
		return
	}

	handle, err := s.runtime.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID: sessionID,
		Message:   params.Message,
		PlanMode:  params.PlanMode,
	})
	if err != nil {
		s.writeError(req.ID, "START_TURN_FAILED", err.Error())
		return
	}
	events, err := s.runtime.Chat().Events(ctx, handle)
	if err != nil {
		s.writeError(req.ID, "EVENTS_FAILED", err.Error())
		return
	}

	translator := agui.NewTranslator(sessionID, handle.TurnID)
	encoder := aguiencoder.NewEventEncoder()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				s.writeDone(req.ID)
				return
			}
			for _, out := range translator.Translate(ev) {
				payload, encErr := encoder.EncodeEvent(ctx, out, "application/json")
				if encErr != nil {
					s.writeError(req.ID, "ENCODE_FAILED", encErr.Error())
					continue
				}
				s.writeEvent(req.ID, payload)
			}
		case <-ctx.Done():
			_ = s.runtime.Chat().Cancel(context.Background(), handle)
			s.writeDone(req.ID)
			return
		}
	}
}

// resolveSession returns threadID after verifying it exists, or
// creates a fresh session when threadID is empty. Mirrors the HTTP
// helper so both transports behave identically.
func (s *Server) resolveSession(ctx context.Context, threadID string) (string, error) {
	if threadID == "" {
		sess, err := s.runtime.Session().Create(ctx, "")
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := s.runtime.Session().Get(ctx, threadID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", errors.New("session not found")
		}
		return "", err
	}
	return threadID, nil
}

// ------------------------------------------------------------------
// agent.steer / agent.cancel — non-streaming control plane
// ------------------------------------------------------------------

type turnIDOnlyParams struct {
	TurnID string `json:"turnId"`
}

type agentSteerParams struct {
	TurnID  string `json:"turnId"`
	Message string `json:"message"`
}

func (s *Server) handleAgentSteer(ctx context.Context, req request) {
	var params agentSteerParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.TurnID == "" || params.Message == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "turnId and message are required")
		return
	}
	if err := s.runtime.Chat().InjectSteering(ctx, chat.TurnHandle{TurnID: params.TurnID}, params.Message); err != nil {
		if errors.Is(err, chat.ErrTurnNotFound) {
			s.writeError(req.ID, "TURN_NOT_FOUND", "turn not found or already ended")
			return
		}
		s.writeError(req.ID, "STEER_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, map[string]bool{"ok": true})
}

func (s *Server) handleAgentCancel(ctx context.Context, req request) {
	var params turnIDOnlyParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.TurnID == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "turnId is required")
		return
	}
	if err := s.runtime.Chat().Cancel(ctx, chat.TurnHandle{TurnID: params.TurnID}); err != nil {
		if errors.Is(err, chat.ErrTurnNotFound) {
			s.writeError(req.ID, "TURN_NOT_FOUND", "turn not found or already ended")
			return
		}
		s.writeError(req.ID, "CANCEL_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, map[string]bool{"ok": true})
}

// ------------------------------------------------------------------
// sessions.* — CRUD
// ------------------------------------------------------------------

type sessionDTO struct {
	ID        string            `json:"id"`
	Title     string            `json:"title,omitempty"`
	ParentID  string            `json:"parent_id,omitempty"`
	StartedAt string            `json:"started_at"`
	UpdatedAt string            `json:"updated_at"`
	TurnCount int               `json:"turn_count"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func toSessionDTO(s session.Session) sessionDTO {
	return sessionDTO{
		ID:        s.ID,
		Title:     s.Title,
		ParentID:  s.ParentID,
		StartedAt: s.StartedAt.UTC().Format(time.RFC3339),
		UpdatedAt: s.UpdatedAt.UTC().Format(time.RFC3339),
		TurnCount: s.TurnCount,
		Metadata:  s.Metadata,
	}
}

func (s *Server) handleSessionsList(ctx context.Context, req request) {
	sessions, err := s.runtime.Session().List(ctx)
	if err != nil {
		s.writeError(req.ID, "LIST_FAILED", err.Error())
		return
	}
	out := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toSessionDTO(sess))
	}
	s.writeResult(req.ID, map[string]any{"sessions": out})
}

type sessionsCreateParams struct {
	Title string `json:"title,omitempty"`
}

func (s *Server) handleSessionsCreate(ctx context.Context, req request) {
	var params sessionsCreateParams
	if !s.decodeParams(req, &params) {
		return
	}
	sess, err := s.runtime.Session().Create(ctx, params.Title)
	if err != nil {
		s.writeError(req.ID, "CREATE_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, toSessionDTO(sess))
}

type sessionsIDParams struct {
	ID string `json:"id"`
}

func (s *Server) handleSessionsGet(ctx context.Context, req request) {
	var params sessionsIDParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.ID == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "id is required")
		return
	}
	sess, err := s.runtime.Session().Get(ctx, params.ID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			s.writeError(req.ID, "NOT_FOUND", "session not found")
			return
		}
		s.writeError(req.ID, "GET_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, toSessionDTO(sess))
}

func (s *Server) handleSessionsDelete(ctx context.Context, req request) {
	var params sessionsIDParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.ID == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "id is required")
		return
	}
	if err := s.runtime.Session().Delete(ctx, params.ID); err != nil {
		s.writeError(req.ID, "DELETE_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, map[string]bool{"ok": true})
}

// ------------------------------------------------------------------
// approvals.* — pending list + decision + mode
// ------------------------------------------------------------------

type approvalDTO struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	ToolName    string `json:"tool_name"`
	Arguments   string `json:"arguments"`
	RequestedAt string `json:"requested_at"`
}

func toApprovalDTO(req approval.Request) approvalDTO {
	return approvalDTO{
		ID:          req.ID,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		ToolName:    req.ToolName,
		Arguments:   req.Arguments,
		RequestedAt: req.RequestedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleApprovalsList(ctx context.Context, req request) {
	pending, err := s.runtime.Approval().ListPending(ctx)
	if err != nil {
		s.writeError(req.ID, "LIST_FAILED", err.Error())
		return
	}
	out := make([]approvalDTO, 0, len(pending))
	for _, p := range pending {
		out = append(out, toApprovalDTO(p))
	}
	s.writeResult(req.ID, map[string]any{"requests": out})
}

type approvalsDecideParams struct {
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"`
}

func (s *Server) handleApprovalsDecide(ctx context.Context, req request) {
	var params approvalsDecideParams
	if !s.decodeParams(req, &params) {
		return
	}
	if params.RequestID == "" {
		s.writeError(req.ID, "INVALID_PARAMS", "requestId is required")
		return
	}
	decision, err := parseDecision(params.Decision)
	if err != nil {
		s.writeError(req.ID, "INVALID_PARAMS", err.Error())
		return
	}
	if err := s.runtime.Approval().Decide(ctx, params.RequestID, decision); err != nil {
		if errors.Is(err, approval.ErrRequestNotFound) {
			s.writeError(req.ID, "NOT_FOUND", "approval request not found")
			return
		}
		s.writeError(req.ID, "DECIDE_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, map[string]bool{"ok": true})
}

type approvalsModeParams struct {
	Mode string `json:"mode"`
}

func (s *Server) handleApprovalsGetMode(ctx context.Context, req request) {
	mode, err := s.runtime.Approval().GetMode(ctx)
	if err != nil {
		s.writeError(req.ID, "GET_MODE_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, approvalsModeParams{Mode: modeName(mode)})
}

func (s *Server) handleApprovalsSetMode(ctx context.Context, req request) {
	var params approvalsModeParams
	if !s.decodeParams(req, &params) {
		return
	}
	mode, err := parseMode(params.Mode)
	if err != nil {
		s.writeError(req.ID, "INVALID_PARAMS", err.Error())
		return
	}
	if err := s.runtime.Approval().SetMode(ctx, mode); err != nil {
		s.writeError(req.ID, "SET_MODE_FAILED", err.Error())
		return
	}
	s.writeResult(req.ID, map[string]bool{"ok": true})
}

// parseDecision / parseMode / modeName are duplicated from the
// HTTP transport. The cost of one extra small file beats sharing
// via an internal-internal helper package — the wire strings are
// the public contract of each transport, not engine-side logic.

func parseDecision(s string) (approval.Decision, error) {
	switch s {
	case "allow_once":
		return approval.DecisionAllowOnce, nil
	case "allow_always":
		return approval.DecisionAllowAlways, nil
	case "deny":
		return approval.DecisionDeny, nil
	default:
		return 0, errors.New(`decision must be one of "allow_once" | "allow_always" | "deny"`)
	}
}

func parseMode(s string) (approval.Mode, error) {
	switch s {
	case "safe":
		return approval.ModeSafe, nil
	case "balanced":
		return approval.ModeBalanced, nil
	case "yolo":
		return approval.ModeYolo, nil
	default:
		return 0, errors.New(`mode must be one of "safe" | "balanced" | "yolo"`)
	}
}

func modeName(m approval.Mode) string {
	switch m {
	case approval.ModeSafe:
		return "safe"
	case approval.ModeBalanced:
		return "balanced"
	case approval.ModeYolo:
		return "yolo"
	default:
		return "unknown"
	}
}

// _jsonRawAlias is purely a compile-time anchor for json.RawMessage
// so removing it from imports doesn't drop the alias when the file
// is edited later.
var _ = json.RawMessage("")
