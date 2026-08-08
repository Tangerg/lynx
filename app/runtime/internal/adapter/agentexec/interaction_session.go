package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type interactionSession struct {
	ref        runs.ExecutorRef
	deployment agent.Deployment
	input      agent.Input
	engine     *agent.Engine
	events     chan runs.ExecutorEvent
	done       chan struct{}
	releasing  chan struct{}

	mu                  sync.Mutex
	process             *agent.Process
	admittedProcessID   agent.ProcessID
	observerWasAttached bool
	begun               bool
	finished            bool
	releaseOnce         sync.Once
	finishOnce          sync.Once
}

func newInteractionSession(
	ref runs.ExecutorRef,
	deployment agent.Deployment,
	input agent.Input,
) *interactionSession {
	return &interactionSession{
		ref: ref, deployment: deployment, input: input,
		events: make(chan runs.ExecutorEvent, interactionEventBuffer),
		done:   make(chan struct{}), releasing: make(chan struct{}),
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

func (session *interactionSession) projectDelta(_ context.Context, delta agent.Delta) {
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
			session.offer(runs.ExecutorEvent{
				Member: runs.ExecutorMember{MemberID: delta.ProcessID().String()}, Payload: payload,
			})
		}
	}
}

func (session *interactionSession) offer(event runs.ExecutorEvent) {
	select {
	case session.events <- event:
	default:
	}
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
	end := segmentEndFromResult(result)
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
		return session.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func segmentEndFromResult(result agent.Result) runs.SegmentEnded {
	termination := result.Termination()
	end := segmentEndFromTermination(
		termination,
		max(result.FinishedAt().Sub(result.StartedAt()), 0),
	)
	if termination.Cause() == agent.TerminationCauseCompletion {
		if erased, ok := result.Output(); ok {
			if output, err := agent.DecodeOutput[interaction.Output](erased); err == nil {
				end.Usage = &runs.SegmentUsage{Steps: int(output.ModelCalls)}
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
