package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// sessionDTO is the JSON shape returned over the wire. Mirrors
// session.Session but with stable JSON tags + ISO timestamps —
// keeps the in-process struct free to evolve without breaking the
// HTTP contract.
type sessionDTO struct {
	ID        string            `json:"id"`
	Title     string            `json:"title,omitempty"`
	ParentID  string            `json:"parent_id,omitempty"`
	StartedAt string            `json:"started_at"`
	UpdatedAt string            `json:"updated_at"`
	TurnCount int               `json:"turn_count"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// toDTO converts a service-layer session to its wire form. Pure
// function — used by every session endpoint that returns one.
func toDTO(s session.Session) sessionDTO {
	return sessionDTO{
		ID:        s.ID,
		Title:     s.Title,
		ParentID:  s.ParentID,
		StartedAt: s.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: s.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		TurnCount: s.TurnCount,
		Metadata:  s.Metadata,
	}
}

// writeJSON marshals body as JSON with the given status. Centralises
// the Content-Type header so each handler stays one expression.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.session.List(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toDTO(sess))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// createSessionRequest is the POST body for /v1/sessions — Title
// is optional; the service defaults to "" which the model later
// fills in.
type createSessionRequest struct {
	Title string `json:"title,omitempty"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var body createSessionRequest
	// Tolerate empty body — title is optional.
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	sess, err := s.session.Create(r.Context(), body.Title)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toDTO(sess))
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id is required")
		return
	}
	sess, err := s.session.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDTO(sess))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.session.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
