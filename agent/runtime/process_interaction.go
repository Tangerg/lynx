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

	var sequence iter.Seq2[toolloop.Event, error]
	resuming := false
	if suspension := p.Suspension(); suspension != nil {
		checkpoint, recognized, checkpointErr := decodeSuspensionCheckpoint(suspension.Payload)
		if checkpointErr != nil {
			return interaction.Result{}, checkpointErr
		}
		if !recognized || checkpoint.Kind != suspensionCheckpointInteraction {
			sequence = runner.Run(ctx, input.Request, input.Tools)
		} else {
			if !suspension.Responded() {
				return interaction.Result{}, fmt.Errorf("%w: tool suspension %q has no response", interaction.ErrSuspensionStale, suspension.ID)
			}
			if checkpoint.Owner != owner {
				return interaction.Result{}, fmt.Errorf("%w: suspension owner %q does not match interaction %q", interaction.ErrSuspensionStale, checkpoint.Owner, owner)
			}
			if checkpoint.Deployment != p.Deployment() {
				return interaction.Result{}, fmt.Errorf("%w: suspension deployment does not match process deployment", interaction.ErrSuspensionStale)
			}
			sequence = runner.Resume(ctx, checkpoint.Checkpoint, input.Tools, toolloop.Resume{ID: suspension.ID, Input: suspension.Response})
			resuming = true
		}
	} else {
		sequence = runner.Run(ctx, input.Request, input.Tools)
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
			if boundary.Pause == nil || boundary.Pause.Checkpoint == nil {
				return interaction.Result{}, errors.New("runtime: tool loop paused without a checkpoint")
			}
			nested, activeNested, err := p.nestedChildrenForCheckpoint(boundary.Pause.Checkpoint)
			if err != nil {
				return interaction.Result{}, fmt.Errorf("runtime: correlate nested child checkpoint: %w", err)
			}
			if activeNested != nil && (activeNested.SuspensionID != boundary.Pause.ID ||
				!bytes.Equal(activeNested.Prompt, boundary.Pause.Prompt) ||
				!bytes.Equal(activeNested.ResumeSchema, boundary.Pause.ResumeSchema)) {
				return interaction.Result{}, fmt.Errorf("%w: nested child pause does not match tool-loop pause", interaction.ErrSuspensionConflict)
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
				return interaction.Result{}, fmt.Errorf("runtime: encode interaction checkpoint: %w", err)
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
			if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, input.Observe); err != nil {
				return interaction.Result{}, err
			}
			return interaction.Result{}, &interaction.SuspendedError{Suspension: suspension}
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
