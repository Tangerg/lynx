package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// agentRunRequest is the POST body for /v1/agent/run. ThreadID
// maps to Lyra's SessionID — the AG-UI nomenclature is "thread";
// internally we call it "session". When ThreadID is empty the
// server creates a fresh session and returns its id via the
// run_started event's threadId.
type agentRunRequest struct {
	ThreadID string `json:"threadId,omitempty"`
	Message  string `json:"message"`
	PlanMode bool   `json:"planMode,omitempty"`
}

// handleAgentRun is the SSE entry point. It:
//
//  1. Parses + validates the request body
//  2. Resolves (or auto-creates) the session
//  3. Starts a Lyra turn and acquires its event channel
//  4. Translates each chat.Event to AG-UI events and writes them
//     as SSE frames using the official AG-UI SSE writer
//
// SSE response headers follow MDN guidance: text/event-stream +
// no-cache + Connection close hint so reverse proxies don't
// buffer. Client disconnect cancels the turn through ctx.
func (s *Server) handleAgentRun(w http.ResponseWriter, r *http.Request) {
	var req agentRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Message == "" {
		writeJSONError(w, http.StatusBadRequest, "message is required")
		return
	}

	sessionID, err := s.resolveSession(r.Context(), req.ThreadID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	handle, err := s.chat().StartTurn(r.Context(), chat.StartTurnRequest{
		SessionID: sessionID,
		Message:   req.Message,
		PlanMode:  req.PlanMode,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := s.chat().Events(r.Context(), handle)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// SSE setup. Header().Set must precede WriteHeader.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	translator := agui.NewTranslator(sessionID, handle.TurnID)
	writer := aguisse.NewSSEWriter()
	s.pumpEvents(r.Context(), w, handle, events, translator, writer)
}

// resolveSession returns the supplied threadID after verifying it
// exists, or creates a fresh session when threadID is empty. The
// "auto-create on empty" path lets thin web clients start a
// conversation without an extra round-trip to /v1/sessions.
func (s *Server) resolveSession(ctx context.Context, threadID string) (string, error) {
	if threadID == "" {
		sess, err := s.session().Create(ctx, "")
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := s.session().Get(ctx, threadID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", errors.New("session not found")
		}
		return "", err
	}
	return threadID, nil
}

// steerRequest is the POST body for /v1/turns/{id}/steer.
type steerRequest struct {
	Message string `json:"message"`
}

// handleSteer queues a mid-turn user message onto an active turn.
// Returns 204 on success, 404 when the turnID is unknown (e.g.
// turn already ended), 400 on bad JSON / empty message.
//
// Semantics: the message lands as conversation history once the
// current turn ends — the chat.Service docs the "next-turn"
// limitation. See chat.Service.InjectSteering.
func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	turnID := r.PathValue("id")
	if turnID == "" {
		writeJSONError(w, http.StatusBadRequest, "turn id is required")
		return
	}
	var body steerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Message == "" {
		writeJSONError(w, http.StatusBadRequest, "message is required")
		return
	}
	// SessionID is unused by chat.InjectSteering's lookup path —
	// the impl indexes turns by TurnID. Pass empty to keep the
	// transport surface minimal.
	err := s.chat().InjectSteering(r.Context(), chat.TurnHandle{TurnID: turnID}, body.Message)
	if err != nil {
		if errors.Is(err, chat.ErrTurnNotFound) {
			writeJSONError(w, http.StatusNotFound, "turn not found or already ended")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pumpEvents drains the per-turn event channel, translates each
// to AG-UI, and writes SSE frames. Stops when the channel closes
// (turn ended) or the client disconnects (ctx cancelled).
//
// Client disconnect cancels the turn through chat.Service.Cancel
// so the runtime doesn't keep running tools no one is listening
// for.
func (s *Server) pumpEvents(
	ctx context.Context,
	w http.ResponseWriter,
	handle chat.TurnHandle,
	events <-chan chat.Event,
	translator *agui.Translator,
	writer *aguisse.SSEWriter,
) {
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			for _, out := range translator.Translate(ev) {
				if err := writer.WriteEvent(ctx, w, out); err != nil {
					_ = s.chat().Cancel(context.Background(), handle)
					return
				}
			}
		case <-ctx.Done():
			_ = s.chat().Cancel(context.Background(), handle)
			return
		}
	}
}
