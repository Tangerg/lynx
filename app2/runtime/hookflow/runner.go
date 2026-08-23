package hookflow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
)

const (
	defaultCommandTimeout = 30 * time.Second
	blockExitCode         = 2
	observationCapacity   = 128
)

type CommandRequest struct {
	Command string
	CWD     string
	Input   lifecyclehook.Invocation
	Timeout time.Duration
}

type CommandDecision struct {
	Verdict          lifecyclehook.Verdict
	Reason           string
	InjectContext    string
	RewriteArguments string
}

type CommandResult struct {
	Decision    CommandDecision
	Stderr      string
	ExitCode    int
	Err         error
	OutputError error
	TimedOut    bool
}

type CommandExecutor interface {
	Execute(context.Context, CommandRequest) CommandResult
}

type observation struct {
	hooks      []lifecyclehook.Hook
	invocation lifecyclehook.Invocation
}

type runner struct {
	commands CommandExecutor
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	jobs     chan observation
	tasks    sync.WaitGroup
	once     sync.Once
}

func newRunner(
	lifetime context.Context,
	commands CommandExecutor,
	logger *slog.Logger,
) *runner {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ctx, cancel := context.WithCancel(lifetime)
	value := &runner{
		commands: commands, logger: logger, ctx: ctx, cancel: cancel,
		jobs: make(chan observation, observationCapacity),
	}
	value.tasks.Add(1)
	go value.work()
	return value
}

func (runner *runner) evaluate(
	ctx context.Context,
	hooks []lifecyclehook.Hook,
	invocation lifecyclehook.Invocation,
) (lifecyclehook.Decision, error) {
	decision := lifecyclehook.Decision{Verdict: lifecyclehook.VerdictAllow}
	toolName := ""
	if invocation.Tool != nil {
		toolName = invocation.Tool.Name
	}
	for _, hook := range hooks {
		if err := ctx.Err(); err != nil {
			return decision, err
		}
		if !hook.Matches(invocation.Event, toolName) {
			continue
		}
		candidate := lifecyclehook.Decision{Verdict: lifecyclehook.VerdictAllow}
		if hook.Inject != "" {
			candidate.Contexts = []lifecyclehook.Context{{
				Event: invocation.Event, Source: hook.Source,
				Content: strings.TrimSpace(hook.Inject),
			}}
		} else {
			result := runner.commands.Execute(ctx, CommandRequest{
				Command: hook.Command, CWD: invocation.Workspace,
				Input: invocation, Timeout: hookTimeout(hook),
			})
			if err := ctx.Err(); err != nil {
				return decision, err
			}
			if result.TimedOut {
				runner.logFailure(ctx, hook, invocation, errors.New("hook command timed out"))
				continue
			}
			if result.ExitCode != 0 && result.ExitCode != blockExitCode {
				runner.logFailure(ctx, hook, invocation, commandFailure(result))
				continue
			}
			if result.ExitCode == 0 && result.Err != nil {
				runner.logFailure(ctx, hook, invocation, result.Err)
				continue
			}
			if result.OutputError != nil {
				runner.logFailure(ctx, hook, invocation, result.OutputError)
				if result.ExitCode != blockExitCode {
					continue
				}
			}
			candidate = commandCandidate(hook, invocation.Event, result)
		}
		if err := decision.Fold(candidate, invocation.Event); err != nil {
			runner.logFailure(ctx, hook, invocation, err)
			candidate.Contexts = nil
			if fallbackErr := decision.Fold(candidate, invocation.Event); fallbackErr != nil {
				runner.logFailure(ctx, hook, invocation, fallbackErr)
			}
		}
	}
	return decision, nil
}

func commandCandidate(
	hook lifecyclehook.Hook,
	event lifecyclehook.Event,
	result CommandResult,
) lifecyclehook.Decision {
	verdict := result.Decision.Verdict
	if verdict == "" {
		verdict = lifecyclehook.VerdictAllow
	}
	if result.ExitCode == blockExitCode {
		verdict = lifecyclehook.VerdictDeny
	}
	reason := boundedText(
		strings.TrimSpace(result.Decision.Reason),
		lifecyclehook.MaxReasonBytes,
	)
	if verdict == lifecyclehook.VerdictDeny && reason == "" {
		reason = boundedText(
			strings.TrimSpace(result.Stderr),
			lifecyclehook.MaxReasonBytes,
		)
	}
	if verdict == lifecyclehook.VerdictDeny && reason == "" {
		reason = "blocked by lifecycle hook"
	}
	contexts := []lifecyclehook.Context(nil)
	if injected := strings.TrimSpace(result.Decision.InjectContext); injected != "" {
		contexts = []lifecyclehook.Context{{Event: event, Source: hook.Source, Content: injected}}
	}
	rewrite := result.Decision.RewriteArguments
	if result.ExitCode == blockExitCode {
		rewrite = ""
	}
	return lifecyclehook.Decision{
		Verdict: verdict, Reason: reason, Contexts: contexts,
		RewriteArguments: rewrite,
	}
}

func commandFailure(result CommandResult) error {
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		return errors.New(stderr)
	}
	if result.Err != nil {
		return result.Err
	}
	return errors.New("hook command exited unsuccessfully")
}

func hookTimeout(hook lifecyclehook.Hook) time.Duration {
	if hook.TimeoutMillis == 0 {
		return defaultCommandTimeout
	}
	return time.Duration(hook.TimeoutMillis) * time.Millisecond
}

func (runner *runner) observe(
	hooks []lifecyclehook.Hook,
	invocation lifecyclehook.Invocation,
) {
	job := observation{
		hooks: slices.Clone(hooks), invocation: cloneInvocation(invocation),
	}
	select {
	case runner.jobs <- job:
	case <-runner.ctx.Done():
	default:
		runner.logFailure(
			context.Background(), lifecyclehook.Hook{}, invocation,
			errors.New("observe-only hook queue is full"),
		)
	}
}

func (runner *runner) work() {
	defer runner.tasks.Done()
	for {
		select {
		case job := <-runner.jobs:
			_, err := runner.evaluate(runner.ctx, job.hooks, job.invocation)
			if err != nil && runner.ctx.Err() == nil {
				runner.logFailure(runner.ctx, lifecyclehook.Hook{}, job.invocation, err)
			}
		case <-runner.ctx.Done():
			return
		}
	}
}

func (runner *runner) logFailure(
	ctx context.Context,
	hook lifecyclehook.Hook,
	invocation lifecyclehook.Invocation,
	err error,
) {
	attributes := []any{
		"event", invocation.Event,
		"session_id", invocation.SessionID,
		"run_id", invocation.RunID,
		"error", err,
	}
	if hook.Source != "" {
		attributes = append(attributes, "source", hook.Source)
	}
	runner.logger.WarnContext(ctx, "lifecycle hook did not complete", attributes...)
}

func (runner *runner) close() {
	runner.once.Do(func() {
		runner.cancel()
		runner.tasks.Wait()
	})
}

func cloneInvocation(value lifecyclehook.Invocation) lifecyclehook.Invocation {
	if value.Tool != nil {
		value.Tool = new(*value.Tool)
	}
	if value.Subagent != nil {
		value.Subagent = new(*value.Subagent)
	}
	return value
}

func boundedText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
