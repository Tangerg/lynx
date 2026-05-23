package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// approvalDTO is the wire shape of [approval.Request]. Kept
// stable across internal refactors of the in-process Request
// struct.
type approvalDTO struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	ToolName    string `json:"tool_name"`
	Arguments   string `json:"arguments"`
	RequestedAt string `json:"requested_at"`
}

func approvalToDTO(req approval.Request) approvalDTO {
	return approvalDTO{
		ID:          req.ID,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		ToolName:    req.ToolName,
		Arguments:   req.Arguments,
		RequestedAt: req.RequestedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	pending, err := s.approval().ListPending(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]approvalDTO, 0, len(pending))
	for _, req := range pending {
		out = append(out, approvalToDTO(req))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// decideRequest is the POST body for /v1/approvals/{id}.
// Decision is one of "allow_once", "allow_always", "deny".
type decideRequest struct {
	Decision string `json:"decision"`
}

func (s *Server) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id is required")
		return
	}
	var body decideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	decision, err := parseDecision(body.Decision)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.approval().Decide(r.Context(), id, decision); err != nil {
		if errors.Is(err, approval.ErrRequestNotFound) {
			writeJSONError(w, http.StatusNotFound, "approval request not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// modeBody is shared between Get and Set on the mode endpoint.
type modeBody struct {
	Mode string `json:"mode"`
}

func (s *Server) handleGetApprovalMode(w http.ResponseWriter, r *http.Request) {
	mode, err := s.approval().GetMode(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, modeBody{Mode: modeName(mode)})
}

func (s *Server) handleSetApprovalMode(w http.ResponseWriter, r *http.Request) {
	var body modeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	mode, err := parseMode(body.Mode)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.approval().SetMode(r.Context(), mode); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseDecision maps the wire-level string to the typed enum.
// Unknown values fail loudly so the client surface tells callers
// up front that the value is wrong.
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
