package agentexec

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type interactionSession struct {
	ref         runs.ExecutorRef
	scope       runs.ExecutionScope
	deployment  agent.Deployment
	input       agent.Input
	engine      *agent.Engine
	events      chan runs.ExecutorEvent
	done        chan struct{}
	releasing   chan struct{}
	unknownWake chan struct{}

	mu                  sync.Mutex
	process             *agent.Process
	admittedProcessID   agent.ProcessID
	observerWasAttached bool
	begun               bool
	finished            bool
	releaseOnce         sync.Once
	finishOnce          sync.Once
	workers             sync.WaitGroup
	usageByModel        map[string]accounting.ModelUsage
	provider            string
	fallbackModel       string
	pricing             accounting.Pricing
	unknownPollInterval time.Duration
	unknownReported     bool
	toolOutcomeKey      string
	toolOutcomeDigest   [sha256.Size]byte
	toolOutcomeRepeats  int
}

func (session *interactionSession) repeatedToolOutcome(toolName string, arguments tool.Arguments) int {
	key := toolName + "\x00" + arguments.Canonical()
	session.mu.Lock()
	defer session.mu.Unlock()
	if key != session.toolOutcomeKey {
		return 0
	}
	return session.toolOutcomeRepeats
}

func (session *interactionSession) resetRepeatedToolOutcome() {
	session.mu.Lock()
	session.toolOutcomeRepeats = 0
	session.mu.Unlock()
}

func (session *interactionSession) recordToolOutcome(
	toolName string,
	arguments tool.Arguments,
	result string,
) {
	key := toolName + "\x00" + arguments.Canonical()
	digest := sha256.Sum256([]byte(result))
	session.mu.Lock()
	defer session.mu.Unlock()
	if key == session.toolOutcomeKey && digest == session.toolOutcomeDigest {
		session.toolOutcomeRepeats++
		return
	}
	session.toolOutcomeKey = key
	session.toolOutcomeDigest = digest
	session.toolOutcomeRepeats = 1
}

func newInteractionSession(
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
	config InteractionExecutorConfig,
) *interactionSession {
	provider := start.ModelSelection.Provider()
	if provider == "" {
		provider = config.Provider
	}
	return &interactionSession{
		ref: ref, scope: rootExecutionScope(start), events: make(chan runs.ExecutorEvent, interactionEventBuffer),
		done: make(chan struct{}), releasing: make(chan struct{}),
		unknownWake: make(chan struct{}, 1), usageByModel: make(map[string]accounting.ModelUsage),
		provider: provider, fallbackModel: start.ModelSelection.Model(), pricing: config.Pricing,
		unknownPollInterval: config.UnknownEffectPollInterval,
	}
}

func (session *interactionSession) attachObserver() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.observerWasAttached || session.begun || session.finished {
		return false
	}
	session.observerWasAttached = true
	return true
}

func (session *interactionSession) observerAttached() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.observerWasAttached
}

func (session *interactionSession) begin() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.begun || session.finished {
		return false
	}
	session.begun = true
	return true
}

func (session *interactionSession) setProcess(process *agent.Process) {
	session.mu.Lock()
	session.process = process
	session.mu.Unlock()
}

func (session *interactionSession) startWorkers() {
	session.workers.Add(2)
	go func() {
		defer session.workers.Done()
		session.await()
	}()
	go func() {
		defer session.workers.Done()
		session.reconcileUnknownEffects()
	}()
}

func (session *interactionSession) failStart() {
	session.finishOnce.Do(func() {
		session.mu.Lock()
		session.finished = true
		session.mu.Unlock()
		close(session.events)
		close(session.done)
	})
}

func (session *interactionSession) admitRoot(_ context.Context, admission agent.ProcessAdmission) error {
	relation := admission.Relation()
	if !admission.Valid() || !relation.IsRoot() || admission.DeploymentRef() != session.deployment.DeploymentRef() {
		return errors.New("agentexec: native Interaction received an invalid root admission")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	processID := relation.ProcessID()
	if session.admittedProcessID.Valid() && session.admittedProcessID != processID {
		return errors.New("agentexec: native Interaction root admission identity changed")
	}
	session.admittedProcessID = processID
	return nil
}

func (session *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		return
	}
	response := parsed.Response()
	for _, choice := range response.Choices {
		if choice.Message == nil {
			continue
		}
		for _, part := range choice.Message.Parts {
			var payload runs.ExecutionFact
			switch part.Kind {
			case corechat.PartText:
				payload = runs.MessageDelta{Text: part.Text}
			case corechat.PartReasoning:
				payload = runs.ReasoningDelta{Text: part.Text}
			default:
				continue
			}
			if session.offer(runs.ExecutorEvent{
				Member: runs.ExecutorMember{MemberID: delta.ProcessID().String()}, Payload: payload,
			}) {
				continue
			}
			trace.SpanFromContext(ctx).AddEvent(
				"agentexec.delta.dropped",
				trace.WithAttributes(attribute.String("process.id", delta.ProcessID().String())),
			)
		}
	}
}

func (session *interactionSession) offer(event runs.ExecutorEvent) bool {
	select {
	case session.events <- event:
		return true
	default:
		return false
	}
}

func (session *interactionSession) commitFact(
	ctx context.Context,
	member runs.ExecutorMember,
	fact runs.ExecutionFact,
) error {
	commit, receipt, err := runs.NewExecutionFactCommit(fact)
	if err != nil {
		return err
	}
	event := runs.ExecutorEvent{Member: member, Payload: commit}
	select {
	case session.events <- event:
	case <-session.releasing:
		return errors.New("agentexec: execution released before authoritative fact commit")
	case <-ctx.Done():
		return ctx.Err()
	}
	return receipt.Await(ctx)
}

func (session *interactionSession) wakeUnknownReconciliation() {
	select {
	case session.unknownWake <- struct{}{}:
	default:
	}
}

func (session *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(session.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.unknownWake:
		case <-ticker.C:
		case <-session.done:
			return
		case <-session.releasing:
			return
		}
		if session.reportUnknownEffects() {
			return
		}
	}
}

func (session *interactionSession) reportUnknownEffects() bool {
	ctx, cancel := context.WithTimeout(context.Background(), authoritativeProjectionTimeout)
	defer cancel()
	ids, err := session.process.UnknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = id.String()
	}
	slices.Sort(values)
	values = slices.Compact(values)
	session.mu.Lock()
	if session.unknownReported {
		session.mu.Unlock()
		return true
	}
	session.unknownReported = true
	member := runs.ExecutorMember{MemberID: session.process.Relation().ProcessID().String()}
	session.mu.Unlock()
	return session.send(runs.ExecutorEvent{
		Member: member, Payload: runs.UnknownEffectsDetected{IDs: values},
	})
}

func (session *interactionSession) accountModelCall(
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) (runs.ModelCallCompleted, error) {
	delta := modelUsage(response, session.provider, session.fallbackModel, session.pricing)
	if err := delta.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	current := session.usageByModel[delta.Model]
	if current.Model == "" {
		current.Model = delta.Model
	}
	current.TokenUsage.Add(delta.TokenUsage)
	current.CostUSD += delta.CostUSD
	current.Calls += delta.Calls
	if err := current.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: aggregate model call: %w", err)
	}
	session.usageByModel[delta.Model] = current
	models := make([]accounting.ModelUsage, 0, len(session.usageByModel))
	for _, usage := range session.usageByModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	total, err := (accounting.Snapshot{Models: models}).Total()
	if err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: total model usage: %w", err)
	}
	if total.Calls != int(invocation.ModelCallSequence()) {
		return runs.ModelCallCompleted{}, fmt.Errorf(
			"agentexec: model call sequence %d differs from accounted calls %d",
			invocation.ModelCallSequence(), total.Calls,
		)
	}
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return runs.ModelCallCompleted{}, errors.New("agentexec: account model call without an assistant message")
	}
	return runs.ModelCallCompleted{
		CallID: callID, Message: choice.Message.Clone(), TokenUsage: total.TokenUsage,
		ByModel: slices.Clone(models), CostUSD: total.CostUSD, Steps: total.Calls,
		ContextTokens: response.Usage.InputTokens,
	}, nil
}

func (session *interactionSession) await() {
	result, err := session.process.Await(context.Background())
	if err == nil {
		err = session.engine.Close()
	}
	if err == nil {
		err = session.publishResult(result)
	}
	if err != nil {
		session.publishProjectionFailure(err)
	}
	session.finishOnce.Do(func() {
		session.mu.Lock()
		session.finished = true
		session.mu.Unlock()
		close(session.events)
		close(session.done)
	})
}

func (session *interactionSession) publishResult(result agent.Result) error {
	member := runs.ExecutorMember{MemberID: result.ProcessID().String()}
	if result.Status() == agent.StatusCompleted {
		erased, ok := result.Output()
		if !ok {
			return errors.New("completed native Interaction has no output")
		}
		output, err := agent.DecodeOutput[interaction.Output](erased)
		if err != nil {
			return fmt.Errorf("decode native Interaction output: %w", err)
		}
		if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil {
			return fmt.Errorf("unsupported native Interaction completion source %q", output.Source)
		}
		choice := output.ModelResponse.First()
		if choice == nil || choice.Message == nil {
			return errors.New("native Interaction output has no assistant message")
		}
		if !session.send(runs.ExecutorEvent{
			Member: member, Payload: runs.AssistantMessageCompleted{Message: choice.Message.Clone()},
		}) {
			return nil
		}
	}
	end := session.segmentEnd(result)
	session.send(runs.ExecutorEvent{Member: member, Payload: end})
	return nil
}

func (session *interactionSession) publishProjectionFailure(_ error) {
	member := runs.ExecutorMember{}
	session.mu.Lock()
	if session.admittedProcessID.Valid() {
		member.MemberID = session.admittedProcessID.String()
	}
	session.mu.Unlock()
	problem := transcript.Problem{
		Kind: transcript.InternalProblem, Scope: transcript.RunProblem,
		Detail: "executor result could not be projected",
	}
	session.send(runs.ExecutorEvent{
		Member:  member,
		Payload: runs.SegmentEnded{Reason: run.OutcomeFailed, Problem: &problem},
	})
}

func (session *interactionSession) send(event runs.ExecutorEvent) bool {
	select {
	case session.events <- event:
		return true
	case <-session.releasing:
		return false
	}
}

func (session *interactionSession) release(ctx context.Context) error {
	session.releaseOnce.Do(func() { close(session.releasing) })
	session.mu.Lock()
	process := session.process
	begun := session.begun
	finished := session.finished
	session.mu.Unlock()
	if !begun {
		session.failStart()
		return session.engine.Close()
	}
	if process != nil && !finished {
		if err := process.Kill(ctx, interactionReleaseReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
			return fmt.Errorf("agentexec: kill native Interaction execution: %w", err)
		}
	}
	select {
	case <-session.done:
		session.workers.Wait()
		return session.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (session *interactionSession) segmentEnd(result agent.Result) runs.SegmentEnded {
	termination := result.Termination()
	end := segmentEndFromTermination(
		termination,
		max(result.FinishedAt().Sub(result.StartedAt()), 0),
	)
	session.mu.Lock()
	models := make([]accounting.ModelUsage, 0, len(session.usageByModel))
	for _, usage := range session.usageByModel {
		models = append(models, usage)
	}
	session.mu.Unlock()
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	if len(models) > 0 {
		if total, err := (accounting.Snapshot{Models: models}).Total(); err == nil {
			end.Usage = &runs.SegmentUsage{
				Tokens: total.TokenUsage, ByModel: models,
				CostUSD: total.CostUSD, Steps: total.Calls,
			}
		}
	}
	return end
}

func segmentEndFromTermination(termination agent.Termination, duration time.Duration) runs.SegmentEnded {
	end := runs.SegmentEnded{Duration: duration}
	switch termination.Cause() {
	case agent.TerminationCauseCompletion:
		end.Reason = run.OutcomeCompleted
	case agent.TerminationCauseProcessDeadline,
		agent.TerminationCauseParentDeadline,
		agent.TerminationCauseHostDeadline:
		end.Reason = run.OutcomeTimedOut
		problem := transcript.Problem{
			Kind: transcript.TimeoutProblem, Scope: transcript.RunProblem,
			Detail: "executor deadline reached",
		}
		end.Problem = &problem
	case agent.TerminationCauseParentCancellation, agent.TerminationCauseHostCancellation:
		end.Reason = run.OutcomeCanceled
	case agent.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		if failure.Code() == "interaction.limit.model_calls" {
			end.Reason = run.OutcomeMaxSteps
			break
		}
		end.Reason = run.OutcomeFailed
		problem := transcript.Problem{
			Kind: transcript.AgentStuckProblem, Scope: transcript.RunProblem,
			Detail: "interaction execution failed",
		}
		end.Problem = &problem
	case agent.TerminationCauseExternalFailure:
		end.Reason = run.OutcomeFailed
		problem := transcript.Problem{
			Kind: transcript.ProviderUnavailableProblem, Scope: transcript.RunProblem,
			Detail: "model provider failed",
		}
		end.Problem = &problem
	case agent.TerminationCauseContractFailure, agent.TerminationCausePanic, agent.TerminationCauseEngineKill:
		end.Reason = run.OutcomeFailed
		problem := transcript.Problem{
			Kind: transcript.InternalProblem, Scope: transcript.RunProblem,
			Detail: "executor failed",
		}
		end.Problem = &problem
	default:
		end.Reason = run.OutcomeFailed
		problem := transcript.Problem{
			Kind: transcript.InternalProblem, Scope: transcript.RunProblem,
			Detail: "executor returned an unknown terminal cause",
		}
		end.Problem = &problem
	}
	return end
}
