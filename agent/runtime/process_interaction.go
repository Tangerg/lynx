package runtime

import (
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

const interactionCheckpointSchemaVersion uint16 = 1

type interactionCheckpoint struct {
	SchemaVersion uint16               `json:"schema_version"`
	Owner         string               `json:"owner"`
	Deployment    core.DeploymentRef   `json:"deployment"`
	Checkpoint    *toolloop.Checkpoint `json:"checkpoint"`
}

func (p *Process) runInteraction(ctx context.Context, actionName string, input core.Interaction) (interaction.Result, error) {
	if err := validateInteraction(input); err != nil {
		return interaction.Result{}, err
	}
	owner, err := interactionOwner(actionName, input)
	if err != nil {
		return interaction.Result{}, err
	}
	runner, err := toolloop.NewRunner(input.Model, toolloop.Config{MaxRounds: input.Limits.MaxRounds})
	if err != nil {
		return interaction.Result{}, err
	}

	var sequence iter.Seq2[toolloop.Event, error]
	resuming := false
	if suspension := p.Suspension(); suspension != nil && suspension.Kind == interaction.SuspensionTool {
		if !suspension.Responded() {
			return interaction.Result{}, fmt.Errorf("%w: tool suspension %q has no response", interaction.ErrSuspensionStale, suspension.ID)
		}
		checkpoint, err := decodeInteractionCheckpoint(suspension.Payload)
		if err != nil {
			return interaction.Result{}, err
		}
		if checkpoint.Owner != owner {
			return interaction.Result{}, fmt.Errorf("%w: suspension owner %q does not match interaction %q", interaction.ErrSuspensionStale, checkpoint.Owner, owner)
		}
		if checkpoint.Deployment != p.Deployment() {
			return interaction.Result{}, fmt.Errorf("%w: suspension deployment does not match process deployment", interaction.ErrSuspensionStale)
		}
		sequence = runner.Resume(ctx, checkpoint.Checkpoint, input.Tools, toolloop.Resume{ID: suspension.ID, Input: suspension.Response})
		resuming = true
	} else {
		sequence = runner.Run(ctx, input.Request, input.Tools)
	}

	modelStarted := map[int]time.Time{}
	unsafeToRetry := false
	interactionFailure := func(err error) error {
		if unsafeToRetry {
			return interaction.Commit(err)
		}
		return err
	}
	for boundary, runErr := range sequence {
		if runErr != nil {
			return interaction.Result{}, interactionFailure(runErr)
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
			p.recordInteractionUsage(ctx, actionName, boundary.Response, time.Since(modelStarted[boundary.Round]), input.Attribute)
			unsafeToRetry = true
		case toolloop.EventToolResult:
			unsafeToRetry = true
			if resuming {
				// The pending tool has now completed. Its checkpoint must never be
				// replayed, even if an observer or later model call fails.
				p.state.clearRespondedSuspension()
				resuming = false
			}
		case toolloop.EventPause:
			if boundary.Pause == nil || boundary.Pause.Checkpoint == nil {
				return interaction.Result{}, interactionFailure(errors.New("runtime: tool loop paused without a checkpoint"))
			}
			payload, err := json.Marshal(interactionCheckpoint{
				SchemaVersion: interactionCheckpointSchemaVersion,
				Owner:         owner,
				Deployment:    p.Deployment(),
				Checkpoint:    boundary.Pause.Checkpoint,
			})
			if err != nil {
				return interaction.Result{}, interactionFailure(fmt.Errorf("runtime: encode interaction checkpoint: %w", err))
			}
			suspension := interaction.Suspension{
				SchemaVersion: interaction.SuspensionSchemaVersion,
				ID:            boundary.Pause.ID,
				Kind:          interaction.SuspensionTool,
				Prompt:        boundary.Pause.Prompt,
				ResumeSchema:  boundary.Pause.ResumeSchema,
				Payload:       payload,
				CreatedAt:     time.Now(),
			}
			frameworkEvent := projectInteractionEvent(boundary, &suspension)
			if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, input.Observe); err != nil {
				return interaction.Result{}, err
			}
			return interaction.Result{}, &interaction.SuspendedError{Suspension: suspension}
		}

		frameworkEvent := projectInteractionEvent(boundary, nil)
		if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent, input.Observe); err != nil {
			return interaction.Result{}, interactionFailure(err)
		}
		if err := ctx.Err(); err != nil {
			return interaction.Result{}, interactionFailure(err)
		}
		if boundary.Final {
			p.state.clearRespondedSuspension()
			return interaction.Result{Final: &frameworkEvent}, nil
		}
	}
	return interaction.Result{}, interactionFailure(errors.New("runtime: managed interaction ended without a final event"))
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
	limits := input.Limits
	if limits.MaxRounds < 0 || limits.MaxSteps < 0 || limits.MaxTokens < 0 || limits.MaxCostUSD < 0 {
		return errors.New("runtime: managed interaction limits must not be negative")
	}
	return nil
}

func interactionOwner(actionName string, input core.Interaction) (string, error) {
	if input.ID != "" {
		return input.ID, nil
	}
	data, err := json.Marshal(struct {
		ActionName string          `json:"action"`
		Request    json.RawMessage `json:"request"`
	}{ActionName: actionName, Request: mustJSON(input.Request)})
	if err != nil {
		return "", fmt.Errorf("runtime: derive interaction owner: %w", err)
	}
	sum := sha256.Sum256(data)
	return "interaction:" + hex.EncodeToString(sum[:]), nil
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func decodeInteractionCheckpoint(payload json.RawMessage) (*interactionCheckpoint, error) {
	var checkpoint interactionCheckpoint
	if err := json.Unmarshal(payload, &checkpoint); err != nil {
		return nil, fmt.Errorf("runtime: decode interaction checkpoint: %w", err)
	}
	if checkpoint.SchemaVersion != interactionCheckpointSchemaVersion || checkpoint.Owner == "" || checkpoint.Checkpoint == nil {
		return nil, errors.New("runtime: invalid interaction checkpoint envelope")
	}
	if err := checkpoint.Deployment.Validate(); err != nil {
		return nil, fmt.Errorf("runtime: interaction checkpoint deployment: %w", err)
	}
	if err := checkpoint.Checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("runtime: interaction checkpoint: %w", err)
	}
	return &checkpoint, nil
}

func (p *Process) interactionStopReason(round int, limits interaction.Limits) interaction.StopReason {
	cost, tokens, _ := p.Usage()
	processBudget := p.options.Budget
	if (processBudget.TokenLimit > 0 && tokens >= processBudget.TokenLimit) ||
		(processBudget.CostLimit > 0 && cost >= processBudget.CostLimit) ||
		(limits.MaxTokens > 0 && int64(tokens) >= limits.MaxTokens) ||
		(limits.MaxCostUSD > 0 && cost >= limits.MaxCostUSD) {
		return interaction.StopBudget
	}
	if limits.MaxSteps > 0 && round-1 >= limits.MaxSteps {
		return interaction.StopSteps
	}
	return interaction.StopNone
}

func (p *Process) recordInteractionUsage(ctx context.Context, actionName string, response *chat.Response, duration time.Duration, attributionFunc core.ModelAttributionFunc) {
	if response == nil {
		return
	}
	usage := response.Usage
	model := response.Model
	if model == "" {
		model = "unknown"
	}
	call := core.ModelCall{
		Model:            model,
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
	processUsage{process: p}.RecordModelCall(ctx, call)
}

func projectInteractionEvent(boundary toolloop.Event, suspension *interaction.Suspension) interaction.Event {
	event := interaction.Event{
		Kind:       interaction.EventKind(boundary.Kind),
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
