package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/media"
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
	userMsg, userMedia, err := collectUserInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	sess, turnReq, err := s.turn.PlanTurnStart(ctx, in.SessionID, s.serverInfo.Cwd, turn.StartTurnRequest{
		Message:    userMsg,
		Media:      userMedia,
		Provider:   in.Provider,
		Model:      in.Model,
		MaxCostUSD: in.MaxBudgetUSD,
		MaxSteps:   in.MaxSteps,
	})
	if err != nil {
		return nil, nil, wireTurnStartErr(err)
	}
	sessionID := sess.ID
	admission, err := s.lifecycle.ClaimRunSlot(ctx, sessionClaimer{s: s}, sessionID)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSessionBusy) {
			return nil, nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, sessionID)
		}
		return nil, nil, err
	}
	defer admission.Release()

	treeAdmission, ok := s.lifecycle.ClaimWorkingTreeRun(sess.Cwd)
	if !ok {
		return nil, nil, fmt.Errorf("%w: working tree %q has a file restore in flight", protocol.ErrSessionBusy, sess.Cwd)
	}
	releaseTreeAdmission := true
	defer func() {
		if releaseTreeAdmission {
			treeAdmission.Release()
		}
	}()

	handle, err := s.turn.StartTurn(ctx, turnReq)
	if err != nil {
		return nil, nil, err
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

func wireTurnStartErr(err error) error {
	switch {
	case errors.Is(err, turn.ErrInputRequired):
		return fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	case errors.Is(err, turn.ErrIncompleteModelSelection):
		return protocol.ErrInvalidParams
	case errors.Is(err, turn.ErrInvalidTurnLimit):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	case errors.Is(err, turn.ErrUnsupportedMedia):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	default:
		return err
	}
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
