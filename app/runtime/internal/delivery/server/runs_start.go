package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	corechat "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/pkg/mime"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// StartRun translates runs.start into the in-process runtime turn
// path (API.md §7.3). It returns the runId synchronously; events flow
// out via the returned channel as RunEvents (wrapped by the transport
// into notifications.run.event). The terminal segment.finished rides this
// channel — including outcome:interrupt when the run parks for HITL
// approval, after which the run suspends and the client answers via
// runs.resume.
func (s *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	options := generationOptionsFromWire(in.Params)
	userMsg, userMedia, err := collectUserInput(in.Input)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.coordinator.Start(ctx, runs.StartCommand{
		SessionID:       in.SessionID,
		DefaultCwd:      s.serverInfo.Cwd,
		Message:         userMsg,
		Media:           userMedia,
		Provider:        in.Provider,
		Model:           in.Model,
		MaxCostUSD:      in.MaxBudgetUSD,
		MaxSteps:        in.MaxSteps,
		Options:         options,
		InterruptKinds:  interruptKindsFromContext(ctx),
		OpeningUserText: userMessageText(in.Input),
		NewProjector:    s.segmentProjector(in.Input),
	})
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: userMessageItemID(result.SegmentID)}, mapRunEvents(ctx, result.Events), nil
}

func wireRunStartErr(err error) error {
	switch {
	case errors.Is(err, runs.ErrInputRequired):
		return fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	case errors.Is(err, runs.ErrIncompleteModelSelection):
		return protocol.ErrInvalidParams
	case errors.Is(err, runs.ErrInvalidTurnLimit):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrInvalidTurnOptions):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrSessionBusy):
		return protocol.ErrSessionBusy
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	default:
		return err
	}
}

func generationOptionsFromWire(in *protocol.GenerationParams) *corechat.Options {
	if in == nil {
		return nil
	}
	return &corechat.Options{
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
		TopP:        in.TopP,
		Stop:        slices.Clone(in.Stop),
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
