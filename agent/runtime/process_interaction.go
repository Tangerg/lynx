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
	"github.com/Tangerg/lynx/agent/internal/panicerr"
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
	model, err := p.managedInteractionModel(input.Stream)
	if err != nil {
		return interaction.Result{}, err
	}
	// An interaction that states no round limit of its own inherits the process
	// limit, which is where a host bounds every managed interaction at once.
	// Unset at both levels means the loop runs until the model stops asking for
	// tools — the framework does not pick a number on the host's behalf.
	rounds := input.Limits.MaxRounds
	if rounds == 0 {
		rounds = p.effectiveMaxToolRounds()
	}
	runner, err := toolloop.NewRunner(model, toolloop.Config{
		MaxRounds:          rounds,
		MaxConcurrentCalls: input.Limits.MaxConcurrentToolCalls,
	})
	if err != nil {
		return interaction.Result{}, err
	}

	sequence, resuming, err := p.resolveInteractionSequence(ctx, runner, input, owner)
	if err != nil {
		return interaction.Result{}, err
	}

	// At most one model-call reservation holds tree capacity at a time, and this
	// loop is its only owner: release settles whatever is still live, and is a
	// no-op once committed. Releasing before the next reservation and on the way
	// out therefore covers every exit path — including ones added later — so
	// reserved capacity can never outlive the call it was admitted for.
	var reservation *modelCallReservation
	defer func() {
		reservation.release()
	}()
	for boundary, runErr := range sequence {
		if runErr != nil {
			// The runner reports its own round bound as an error because an event
			// sequence has no other channel. A managed interaction does have one,
			// and the bound came from the host, so it is reported as the outcome
			// it is instead of failing the process.
			if errors.Is(runErr, toolloop.ErrRoundLimit) {
				return interaction.Result{StopReason: interaction.StopSteps}, nil
			}
			return interaction.Result{}, runErr
		}
		if boundary.Kind == toolloop.EventModelRequest {
			frameworkEvent := projectInteractionEvent(boundary, nil, 0)
			if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent); err != nil {
				return interaction.Result{}, err
			}
			if err := ctx.Err(); err != nil {
				return interaction.Result{}, err
			}
			reservation.release()
			var stop interaction.StopReason
			reservation, stop, err = p.budget.reserveModelCall()
			if err != nil {
				return interaction.Result{}, fmt.Errorf("runtime: reserve model call: %w", err)
			}
			if stop != interaction.StopNone {
				return interaction.Result{StopReason: stop}, nil
			}
			continue
		}

		var (
			recordedUsage core.Usage
			transitionErr error
		)
		switch boundary.Kind {
		case toolloop.EventModelResponse:
			recordedUsage, transitionErr = p.recordInteractionUsage(ctx, boundary.Response, reservation)
			if transitionErr != nil {
				transitionErr = fmt.Errorf("runtime: record interaction usage: %w", transitionErr)
			}
		case toolloop.EventToolResult:
			if resuming {
				// The pending tool has completed. Remove its checkpoint so a later
				// continuation cannot execute the completed call again.
				p.state.clearRespondedSuspension()
				resuming = false
			}
		case toolloop.EventPause:
			return interaction.Result{}, p.pauseInteraction(ctx, boundary, owner)
		}

		frameworkEvent := projectInteractionEvent(boundary, nil, recordedUsage.Cost)
		publishErr := p.publishInteractionBoundary(ctx, owner, frameworkEvent)
		if transitionErr != nil || publishErr != nil {
			return interaction.Result{}, errors.Join(transitionErr, publishErr)
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

func (p *Process) managedInteractionModel(stream func(*chat.Response)) (chat.Model, error) {
	capability, err := p.effectiveChat()
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve managed interaction model: %w", err)
	}
	if capability.Model == nil {
		return nil, errors.New("runtime: managed interaction has no configured chat model")
	}
	if stream == nil {
		return capability.Model, nil
	}
	if capability.Streamer == nil {
		return nil, errors.New("runtime: managed interaction requested streaming but the configured chat capability has no streamer")
	}
	return managedStreamModel{streamer: capability.Streamer, observe: stream}, nil
}

type managedStreamModel struct {
	streamer chat.Streamer
	observe  func(*chat.Response)
}

func (m managedStreamModel) Call(ctx context.Context, request *chat.Request) (response *chat.Response, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = nil
			err = panicerr.New("runtime: managed interaction stream observer panicked", recovered)
		}
	}()
	return interaction.StreamCall(ctx, m.streamer, request, func(delta *chat.Response) {
		m.observe(delta.Clone())
	})
}

// resolveInteractionSequence starts a fresh tool loop, or resumes the pending
// one when this process was suspended inside a managed interaction. resuming is
// true only on the resume path, so the caller can drop the responded suspension
// once the pending tool result arrives. Empty framework state and nested-child
// state belong to other continuation paths and start a fresh interaction;
// malformed framework state fails closed.
func (p *Process) resolveInteractionSequence(ctx context.Context, runner *toolloop.Runner, input core.Interaction, owner string) (iter.Seq2[toolloop.Event, error], bool, error) {
	suspension := p.Suspension()
	if suspension == nil {
		return runner.Run(ctx, input.Request, input.Tools), false, nil
	}
	checkpoint, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return nil, false, err
	}
	if checkpoint == nil || checkpoint.Kind != suspensionCheckpointInteraction {
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

// pauseInteraction records the tool-loop checkpoint as a process suspension,
// publishes the pause boundary, and returns the SuspendedError that unwinds the
// action. It reconciles the tool-loop pause with an active nested-child pause so
// the two cannot disagree on suspension identity, prompt, or schema.
func (p *Process) pauseInteraction(ctx context.Context, boundary toolloop.Event, owner string) error {
	if boundary.Pause == nil || boundary.Pause.Checkpoint == nil {
		return errors.New("runtime: tool loop paused without a checkpoint")
	}
	nested, activeNested, err := p.nestedChildrenForCheckpoint(boundary.Pause.Checkpoint)
	if err != nil {
		return fmt.Errorf("runtime: correlate nested child checkpoint: %w", err)
	}
	// When the paused call is served by a child, the two subsystems must agree on
	// what is being waited for. The child owns that fact, so the comparison reads
	// its live suspension rather than a copy kept beside the relation.
	var activeChild *interaction.Suspension
	if activeNested != nil {
		activeChild, err = p.nestedChildSuspension(activeNested)
		if err != nil {
			return err
		}
		if activeChild.ID != boundary.Pause.ID ||
			!bytes.Equal(activeChild.Prompt, boundary.Pause.Prompt) ||
			!bytes.Equal(activeChild.ResumeSchema, boundary.Pause.ResumeSchema) {
			return fmt.Errorf("%w: nested child pause does not match tool-loop pause", interaction.ErrSuspensionConflict)
		}
	}
	frameworkState, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
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
	// A promoted child suspension keeps the child's creation time so the parent
	// exposes when the wait actually began.
	createdAt := time.Now()
	if activeChild != nil {
		createdAt = activeChild.CreatedAt
	}
	suspension := interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             boundary.Pause.ID,
		Prompt:         boundary.Pause.Prompt,
		ResumeSchema:   boundary.Pause.ResumeSchema,
		FrameworkState: frameworkState,
		CreatedAt:      createdAt,
	}
	frameworkEvent := projectInteractionEvent(boundary, &suspension, 0)
	if err := p.publishInteractionBoundary(ctx, owner, frameworkEvent); err != nil {
		return err
	}
	return &interaction.SuspendedError{Suspension: suspension}
}

func validateInteraction(input core.Interaction) error {
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

// recordInteractionUsage commits the reservation admitted for this model call
// with the usage the response reported. It never releases: the interaction loop
// owns that half of the reservation lifecycle, so every failure exit frees the
// capacity without this function having to anticipate it.
func (p *Process) recordInteractionUsage(
	ctx context.Context,
	response *chat.Response,
	reservation *modelCallReservation,
) (core.Usage, error) {
	if response == nil {
		return core.Usage{}, nil
	}
	tokens, err := usageTokenCount(response.Usage.InputTokens, response.Usage.OutputTokens)
	if err != nil {
		return core.Usage{}, err
	}
	usage := core.Usage{Tokens: tokens, ModelCalls: 1}
	usage.Cost, err = p.projectInteractionCost(ctx, response)
	if err != nil {
		usage.Cost = 0
		if commitErr := reservation.commit(usage); commitErr != nil {
			return core.Usage{}, errors.Join(err, commitErr)
		}
		return usage, err
	}
	if err := reservation.commit(usage); err != nil {
		return core.Usage{}, err
	}
	return usage, nil
}

func (p *Process) projectInteractionCost(ctx context.Context, response *chat.Response) (float64, error) {
	projector, ok := firstExtension[core.InteractionCostProjector](p.combinedExtensionsResolverFirst())
	if !ok {
		return 0, nil
	}
	cost, err := callInteractionCostProjector(ctx, projector, p, response)
	if err != nil {
		return 0, err
	}
	usage := core.Usage{Cost: cost}
	if err := usage.Validate(); err != nil {
		return 0, err
	}
	return cost, nil
}

func callInteractionCostProjector(
	ctx context.Context,
	projector extensionCapability[core.InteractionCostProjector],
	process core.ProcessView,
	response *chat.Response,
) (cost float64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("interaction cost projector %q panicked", projector.name), recovered)
		}
	}()
	return projector.value.ProjectInteractionCost(ctx, process, response.Clone())
}

func projectInteractionEvent(boundary toolloop.Event, suspension *interaction.Suspension, cost float64) interaction.Event {
	event := interaction.Event{
		Kind:       boundary.Kind,
		Round:      boundary.Round,
		Final:      boundary.Final,
		Cost:       cost,
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

func (p *Process) publishInteractionBoundary(ctx context.Context, owner string, boundary interaction.Event) error {
	if err := boundary.Validate(); err != nil {
		return fmt.Errorf("runtime: project interaction event: %w", err)
	}
	p.publishEvent(ctx, event.InteractionBoundary{
		Header:        p.eventHeader(),
		Deployment:    p.Deployment(),
		InteractionID: owner,
		Boundary:      boundary.Clone(),
	})
	return nil
}
