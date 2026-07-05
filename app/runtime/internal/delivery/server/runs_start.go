package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
	"github.com/Tangerg/lynx/pkg/mime"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// StartRun translates runs.start into the in-process runtime turn
// path (API.md §7.3). It returns the runId synchronously; events flow
// out via the returned channel as RunEvents (wrapped by the transport
// into notifications.run.event). The terminal run.finished rides this
// channel — including outcome:interrupt when the run parks for HITL
// approval, after which the run suspends and the client answers via
// runs.resume.
func (s *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	sessionID, err := s.resolveSession(ctx, in.SessionID)
	if err != nil {
		return nil, nil, err
	}

	// The turn's filesystem + shell tools run in the session's project cwd
	// (API.md §0.2). Resolve it here so the engine anchors them per session
	// rather than at the single serve-time workdir.
	sess, err := s.rt.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	userMsg, userMedia, err := collectUserInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	if userMsg == "" && len(userMedia) == 0 {
		return nil, nil, fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	}

	// providerId + model are paired: both to pick a model, neither for the
	// default. One without the other is a self-contradictory request — the
	// provider is explicit, never inferred (API.md §7.3).
	if (in.Model == "") != (in.Provider == "") {
		return nil, nil, protocol.ErrInvalidParams
	}

	admission, err := s.rt.ClaimRunSlot(ctx, sessionClaimer{s: s}, sessionID)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSessionBusy) {
			return nil, nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, sessionID)
		}
		return nil, nil, err
	}
	defer admission.Release()

	// Image capability gate: when the run names an explicit provider+model,
	// refuse image input the model can't accept (catalog modalities) so the
	// user gets a clear error up front instead of a provider 400 mid-stream.
	// The default model (provider+model empty) is operator-configured and
	// skipped here; the frontend also gates via the per-model multimodal
	// capability (models.list). A catalog miss means unknown — let the
	// provider decide rather than block.
	if len(userMedia) > 0 && in.Provider != "" && in.Model != "" {
		if info, ok := catalog.Lookup(in.Provider, in.Model); ok && !info.Modalities.AcceptsInput(chat.ModalityImage) {
			return nil, nil, fmt.Errorf("%w: model %q (provider %q) does not accept image input", protocol.ErrInvalidParams, in.Model, in.Provider)
		}
	}

	treeAdmission, ok := s.claimWorkingTreeRun(sess.Cwd)
	if !ok {
		return nil, nil, fmt.Errorf("%w: working tree %q has a file restore in flight", protocol.ErrSessionBusy, sess.Cwd)
	}
	releaseTreeAdmission := true
	defer func() {
		if releaseTreeAdmission {
			treeAdmission.Release()
		}
	}()

	handle, err := s.rt.StartTurn(ctx, turn.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		Media:      userMedia,
		Cwd:        sess.Cwd,
		Provider:   in.Provider,
		Model:      in.Model,
		MaxCostUSD: in.MaxBudgetUSD,
		MaxSteps:   in.MaxSteps,
	})
	if err != nil {
		return nil, nil, err
	}

	// Record the model the run explicitly selected so sessions.list / get
	// surface the session's current model (Session.model). An unset model
	// runs the default — sessionToWire fills that from the runtime default.
	if in.Model != "" {
		_ = s.rt.SetSessionModel(ctx, sessionID, in.Model)
	}

	// runId on the wire == the turn id for the root run. The user's input
	// rides the stream as the run's opening userMessage Item (translator
	// emits it after run.started) — streamed live and persisted through the
	// same path, so the wire id and the items.list id are one and the same.
	runID := handle.TurnID
	out, events, err := s.openSegment(ctx, runID, "", handle, sessionID, in.Input, nil, in.Provider, in.Model)
	if err != nil {
		return nil, nil, err
	}
	treeAdmission.Release()
	releaseTreeAdmission = false
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	out.UserItemID = userMessageItemID(runID)
	return out, events, nil
}

// resolveSession verifies sessionID exists, or creates a fresh session
// when empty (zero-friction cold start for in-process callers).
func (s *Server) resolveSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := s.rt.CreateSession(ctx, "", s.serverInfo.Cwd)
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := s.rt.GetSession(ctx, sessionID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", protocol.ErrSessionNotFound
		}
		return "", err
	}
	return sessionID, nil
}

// collectUserInput splits a run's input blocks into the turn's user message:
// all text blocks joined newline-separated, and all image blocks turned into
// core media (Mime parsed to a MIME, Data taken as the base64 payload). An
// image block with a missing / non-image mime or empty data is rejected as
// invalid_params rather than silently dropped, so a malformed attachment
// surfaces to the user instead of vanishing. Unknown block types are ignored
// (forward-compatible). Media flows to the model as UserMessage.Media; the
// original blocks ride the opening userMessage Item verbatim for echo/replay.
func collectUserInput(blocks []protocol.ContentBlock) (string, []*media.Media, error) {
	var (
		texts  []string
		images []*media.Media
	)
	for _, blk := range blocks {
		switch blk.Type {
		case protocol.ContentBlockText:
			if blk.Text != "" {
				texts = append(texts, blk.Text)
			}
		case protocol.ContentBlockImage:
			mt, err := mime.Parse(blk.Mime)
			if err != nil {
				return "", nil, fmt.Errorf("%w: image block has invalid mime %q", protocol.ErrUnsupportedMime, blk.Mime)
			}
			if !mime.IsImage(mt) {
				return "", nil, fmt.Errorf("%w: block mime %q is not an image type", protocol.ErrUnsupportedMime, blk.Mime)
			}
			if blk.Data == "" {
				return "", nil, fmt.Errorf("%w: image block has empty data", protocol.ErrInvalidParams)
			}
			// Data is the base64 payload as a string — the form both the
			// anthropic (NewImageBlockBase64) and openai-compatible adapters
			// read back via Media.DataAsString.
			m, err := media.NewMedia(mt, blk.Data)
			if err != nil {
				return "", nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
			}
			images = append(images, m)
		}
	}
	return strings.Join(texts, "\n"), images, nil
}
