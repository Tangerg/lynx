package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

const derivedInteractionIDPrefix = "interaction:"

func (p *Process) runInteraction(ctx context.Context, actionName string, input core.Interaction) (interaction.Result, error) {
	if err := validateInteraction(input); err != nil {
		return interaction.Result{}, err
	}
	owner, err := p.interactionOwner(actionName, input)
	if err != nil {
		return interaction.Result{}, err
	}
	runner, err := toolloop.NewRunner(input.Model, toolloop.Config{
		MaxRounds:          input.Limits.MaxRounds,
		MaxConcurrentCalls: input.Limits.MaxConcurrentToolCalls,
	})
	if err != nil {
		return interaction.Result{}, err
	}

	sequence, resuming, err := p.resolveInteractionSequence(ctx, runner, input, owner)
	if err != nil {
		return interaction.Result{}, err
	}

	modelStarted := map[int]time.Time{}
	for boundary, runErr := range sequence {
		if runErr != nil {
			return interaction.Result{}, runErr
		}
		if boundary.Kind == toolloop.EventModelRequest {
			frameworkEvent := projectInteractionEvent(boundary, nil)
			if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, input.Observe); err != nil {
				return interaction.Result{}, err
			}
			if err := ctx.Err(); err != nil {
				return interaction.Result{}, err
			}
			if stop := p.interactionStopReason(boundary.Round, input.Limits); stop != interaction.StopNone {
				return interaction.Result{StopReason: stop}, nil
			}
			modelStarted[boundary.Round] = time.Now()
			continue
		}

		switch boundary.Kind {
		case toolloop.EventModelResponse:
			if err := p.recordInteractionUsage(ctx, actionName, boundary.Response, time.Since(modelStarted[boundary.Round]), input.Attribute); err != nil {
				return interaction.Result{}, fmt.Errorf("runtime: record interaction usage: %w", err)
			}
		case toolloop.EventToolResult:
			if resuming {
				// The pending tool has completed. Remove its checkpoint so a later
				// continuation cannot execute the completed call again.
				p.state.clearRespondedSuspension()
				resuming = false
			}
		case toolloop.EventPause:
			return interaction.Result{}, p.pauseInteraction(ctx, boundary, owner, input.Observe)
		}

		frameworkEvent := projectInteractionEvent(boundary, nil)
		if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, input.Observe); err != nil {
			return interaction.Result{}, err
		}
		if err := ctx.Err(); err != nil {
			return interaction.Result{}, err
		}
		if boundary.Final {
			p.state.clearRespondedSuspension()
			return interaction.Result{Final: &frameworkEvent}, nil
		}
	}
	return interaction.Result{}, errors.New("runtime: managed interaction ended without a final event")
}

// resolveInteractionSequence starts a fresh tool loop, or resumes the pending
// one when this process was suspended inside a managed interaction. resuming is
// true only on the resume path, so the caller can drop the responded suspension
// once the pending tool result arrives. A suspension whose checkpoint is not a
// recognized interaction checkpoint starts fresh rather than failing.
func (p *Process) resolveInteractionSequence(ctx context.Context, runner *toolloop.Runner, input core.Interaction, owner string) (iter.Seq2[toolloop.Event, error], bool, error) {
	suspension := p.Suspension()
	if suspension == nil {
		return runner.Run(ctx, input.Request, input.Tools), false, nil
	}
	checkpoint, recognized, err := decodeSuspensionCheckpoint(suspension.Payload)
	if err != nil {
		return nil, false, err
	}
	if !recognized || checkpoint.Kind != suspensionCheckpointInteraction {
		return runner.Run(ctx, input.Request, input.Tools), false, nil
	}
	if !suspension.Responded() {
		return nil, false, fmt.Errorf("%w: tool suspension %q has no response", interaction.ErrSuspensionStale, suspension.ID)
	}
	if checkpoint.Owner != owner {
		return nil, false, fmt.Errorf("%w: suspension owner %q does not match interaction %q", interaction.ErrSuspensionStale, checkpoint.Owner, owner)
	}
	if checkpoint.Deployment != p.Deployment() {
		return nil, false, fmt.Errorf("%w: suspension deployment does not match process deployment", interaction.ErrSuspensionStale)
	}
	resume := toolloop.Resume{ID: suspension.ID, Input: suspension.Response}
	return runner.Resume(ctx, checkpoint.Checkpoint, input.Tools, resume), true, nil
}

// pauseInteraction persists the tool-loop checkpoint as a process suspension,
// publishes the pause boundary, and returns the SuspendedError that unwinds the
// action. It reconciles the tool-loop pause with an active nested-child pause so
// the two cannot disagree on suspension identity, prompt, or schema.
func (p *Process) pauseInteraction(ctx context.Context, boundary toolloop.Event, owner string, observe interaction.Observer) error {
	if boundary.Pause == nil || boundary.Pause.Checkpoint == nil {
		return errors.New("runtime: tool loop paused without a checkpoint")
	}
	nested, activeNested, err := p.nestedChildrenForCheckpoint(boundary.Pause.Checkpoint)
	if err != nil {
		return fmt.Errorf("runtime: correlate nested child checkpoint: %w", err)
	}
	if activeNested != nil && (activeNested.SuspensionID != boundary.Pause.ID ||
		!bytes.Equal(activeNested.Prompt, boundary.Pause.Prompt) ||
		!bytes.Equal(activeNested.ResumeSchema, boundary.Pause.ResumeSchema)) {
		return fmt.Errorf("%w: nested child pause does not match tool-loop pause", interaction.ErrSuspensionConflict)
	}
	payload, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion:  suspensionCheckpointSchemaVersion,
		Kind:           suspensionCheckpointInteraction,
		Owner:          owner,
		Deployment:     p.Deployment(),
		Checkpoint:     boundary.Pause.Checkpoint,
		NestedChildren: nested,
	})
	if err != nil {
		return fmt.Errorf("runtime: encode interaction checkpoint: %w", err)
	}
	kind := interaction.SuspensionTool
	createdAt := time.Now()
	if activeNested != nil {
		kind = activeNested.SuspensionKind
		createdAt = activeNested.SuspensionCreatedAt
	}
	suspension := interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            boundary.Pause.ID,
		Kind:          kind,
		Prompt:        boundary.Pause.Prompt,
		ResumeSchema:  boundary.Pause.ResumeSchema,
		Payload:       payload,
		CreatedAt:     createdAt,
	}
	frameworkEvent := projectInteractionEvent(boundary, &suspension)
	if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, observe); err != nil {
		return err
	}
	return &interaction.SuspendedError{Suspension: suspension}
}

func validateInteraction(input core.Interaction) error {
	if input.Model == nil {
		return errors.New("runtime: managed interaction model is nil")
	}
	if input.Request == nil {
		return errors.New("runtime: managed interaction request is nil")
	}
	if err := input.Request.Validate(); err != nil {
		return fmt.Errorf("runtime: managed interaction request: %w", err)
	}
	if strings.TrimSpace(input.ID) != input.ID {
		return errors.New("runtime: managed interaction ID has surrounding whitespace")
	}
	if err := input.Limits.Validate(); err != nil {
		return fmt.Errorf("runtime: managed interaction: %w", err)
	}
	return nil
}

func (p *Process) interactionOwner(actionName string, input core.Interaction) (string, error) {
	if input.ID != "" {
		return input.ID, nil
	}
	data, err := json.Marshal(struct {
		ProcessID  string        `json:"process_id"`
		ActionName string        `json:"action"`
		Request    *chat.Request `json:"request"`
	}{
		ProcessID:  p.ID(),
		ActionName: actionName,
		Request:    input.Request,
	})
	if err != nil {
		return "", fmt.Errorf("runtime: derive interaction owner: %w", err)
	}
	sum := sha256.Sum256(data)
	return derivedInteractionIDPrefix + hex.EncodeToString(sum[:]), nil
}

func (p *Process) interactionStopReason(round int, limits interaction.Limits) interaction.StopReason {
	cost, tokens, _ := p.Usage()
	processBudget := p.options.budget
	if (processBudget.TokenLimit > 0 && tokens >= processBudget.TokenLimit) ||
		(processBudget.CostLimit > 0 && cost >= processBudget.CostLimit) ||
		(limits.MaxTokens > 0 && int64(tokens) >= limits.MaxTokens) ||
		(limits.MaxCostUSD > 0 && cost >= limits.MaxCostUSD) {
		return interaction.StopBudget
	}
	if limits.MaxModelCalls > 0 && len(p.ModelCalls()) >= limits.MaxModelCalls {
		return interaction.StopSteps
	}
	if limits.MaxSteps > 0 && round-1 >= limits.MaxSteps {
		return interaction.StopSteps
	}
	return interaction.StopNone
}

func (p *Process) recordInteractionUsage(ctx context.Context, actionName string, response *chat.Response, duration time.Duration, attributionFunc core.ModelAttributionFunc) error {
	if response == nil {
		return nil
	}
	usage := response.Usage
	call := core.ModelCall{
		Model:            response.Model,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		Duration:         duration,
		ActionName:       actionName,
	}
	if usage.ReasoningTokens != nil {
		call.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadInputTokens != nil {
		call.CacheReadInputTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheWriteInputTokens != nil {
		call.CacheWriteInputTokens = *usage.CacheWriteInputTokens
	}
	if attributionFunc != nil {
		attribution := attributionFunc(response)
		call.Provider = attribution.Provider
		call.CostUSD = attribution.CostUSD
	}
	return processUsage{process: p}.RecordModelCall(ctx, call)
}

func projectInteractionEvent(boundary toolloop.Event, suspension *interaction.Suspension) interaction.Event {
	event := interaction.Event{
		Kind:       boundary.Kind,
		Round:      boundary.Round,
		Final:      boundary.Final,
		Request:    boundary.Request,
		Response:   boundary.Response,
		ToolCall:   boundary.ToolCall,
		ToolResult: boundary.ToolResult,
		Suspension: suspension,
	}
	if boundary.Resume != nil {
		event.Resume = &interaction.Resume{ID: boundary.Resume.ID, Input: boundary.Resume.Input}
	}
	return event
}

func (p *Process) publishInteractionBoundary(ctx context.Context, owner string, boundary interaction.Event, observer interaction.Observer) error {
	if err := boundary.Validate(); err != nil {
		return fmt.Errorf("runtime: project interaction event: %w", err)
	}
	p.publishEvent(ctx, event.InteractionBoundary{
		Header:        p.eventHeader(),
		Deployment:    p.Deployment(),
		InteractionID: owner,
		Boundary:      boundary,
	})
	if observer != nil {
		if err := observer(ctx, boundary); err != nil {
			return fmt.Errorf("runtime: interaction observer: %w", err)
		}
	}
	return nil
}
