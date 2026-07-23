package server

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corechat "github.com/Tangerg/lynx/core/chat"

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
	result, err := s.coordinator.Start(ctx, runs.StartCommand{
		SessionID:      in.SessionID,
		DefaultCwd:     s.serverInfo.Cwd,
		Provider:       in.Provider,
		Model:          in.Model,
		MaxCostUSD:     in.MaxBudgetUSD,
		MaxSteps:       in.MaxSteps,
		Options:        options,
		InterruptKinds: interruptKindsFromContext(ctx),
		Input:          runInputFromWire(in.Input),
	})
	if err != nil {
		return nil, nil, wireRunStartErr(err)
	}
	// Return the opening userMessage Item id so the client reconciles its
	// optimistic bubble by exact id (same id the stream + items.list carry).
	return &protocol.StartRunResponse{RunID: result.RunID, SegmentID: result.SegmentID, UserItemID: result.UserItemID}, mapRunEvents(ctx, result.Events), nil
}

func runInputFromWire(blocks []protocol.ContentBlock) []runs.ContentBlock {
	input := make([]runs.ContentBlock, len(blocks))
	for i, block := range blocks {
		kind := runs.TextContent
		if block.Type == protocol.ContentBlockImage {
			kind = runs.ImageContent
		}
		input[i] = runs.ContentBlock{Kind: kind, Text: block.Text, Mime: block.Mime, Data: block.Data}
	}
	return input
}

func wireRunStartErr(err error) error {
	switch {
	case errors.Is(err, runs.ErrInputRequired):
		return fmt.Errorf("%w: input must contain a user text or image block", protocol.ErrInvalidParams)
	case errors.Is(err, runs.ErrIncompleteModelSelection):
		return protocol.ErrInvalidParams
	case errors.Is(err, runs.ErrInvalidTurnLimit):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrInvalidTurnOptions):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, runs.ErrUnsupportedMedia):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
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
