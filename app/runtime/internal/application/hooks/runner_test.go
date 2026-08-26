package hooks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	hookdomain "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func ctxBG() context.Context { return context.Background() }

type commandStub struct {
	mu       sync.Mutex
	results  []CommandResult
	requests []CommandRequest
}

func (c *commandStub) RunHookCommand(_ context.Context, req CommandRequest) CommandResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if len(c.results) == 0 {
		return CommandResult{}
	}
	out := c.results[0]
	c.results = c.results[1:]
	return out
}

func (c *commandStub) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func TestRunner_DeclarativeInject(t *testing.T) {
	r := NewRunner(nil, nil)
	hooks := []hookdomain.Hook{{Event: hookdomain.SessionStart, Inject: "remember: use tabs"}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.SessionStart})
	if dec.InjectContext != "remember: use tabs" {
		t.Fatalf("InjectContext = %q", dec.InjectContext)
	}
	if dec.Block {
		t.Error("declarative inject should not block")
	}
}

func TestRunner_CommandReceivesTypedEvent(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandAllow, InjectContext: "saw-event"}}}}
	r := NewRunner(cmds, nil)
	hooks := []hookdomain.Hook{{
		Event:   hookdomain.UserPromptSubmit,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.UserPromptSubmit, Prompt: "hi"})
	if dec.InjectContext != "saw-event" {
		t.Fatalf("InjectContext = %q — stdin event not delivered?", dec.InjectContext)
	}
	if len(cmds.requests) != 1 || cmds.requests[0].Input.Event != hookdomain.UserPromptSubmit || cmds.requests[0].Input.Prompt != "hi" {
		t.Fatalf("request = %+v, want typed prompt event", cmds.requests)
	}
}

func TestRunnerProjectsBoundedMaterialForCommandHooks(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandAllow}}}}
	r := NewRunner(cmds, nil)
	r.Run(ctxBG(), []hookdomain.Hook{{
		Event: hookdomain.UserPromptSubmit, Command: "hook",
	}}, hookdomain.Input{
		Event:  hookdomain.UserPromptSubmit,
		Prompt: strings.Repeat("p", hookdomain.MaxPromptBytes+1),
	})
	if len(cmds.requests) != 1 {
		t.Fatalf("command requests = %d, want 1", len(cmds.requests))
	}
	got := cmds.requests[0].Input
	if len(got.Prompt) != hookdomain.MaxPromptBytes || !got.PromptTruncated {
		t.Fatalf("projected prompt = %d bytes truncated=%v", len(got.Prompt), got.PromptTruncated)
	}
}

func TestRunnerRejectsLossyArgumentsWithoutDroppingDeclarativeContext(t *testing.T) {
	cmds := &commandStub{}
	var observed error
	r := NewRunner(cmds, func(_ context.Context, _ string, err error) {
		observed = err
	})
	decision := r.Run(ctxBG(), []hookdomain.Hook{
		{Event: hookdomain.PreToolUse, Inject: "bounded context"},
		{Event: hookdomain.PreToolUse, Command: "hook", Source: "/tmp/hooks.json"},
	}, hookdomain.Input{
		Event: hookdomain.PreToolUse,
		Tool: &hookdomain.ToolInput{
			Name: "shell", Arguments: strings.Repeat("a", hookdomain.MaxArgumentsBytes+1),
		},
	})
	if cmds.calls() != 0 || observed == nil {
		t.Fatalf("command calls = %d observed=%v, want rejected observable command input", cmds.calls(), observed)
	}
	if decision.InjectContext != "bounded context" {
		t.Fatalf("declarative context = %q, want preserved bounded path", decision.InjectContext)
	}
}

func TestRunner_StdoutDenyBlocks(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandDeny, Reason: "no rm allowed"}}}}, nil)
	hooks := []hookdomain.Hook{{
		Event:   hookdomain.PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if !dec.Block || dec.Reason != "no rm allowed" {
		t.Fatalf("got block=%v reason=%q, want deny", dec.Block, dec.Reason)
	}
}

func TestRunner_Exit2Blocks(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{
		Stderr:   "blocked by policy",
		ExitCode: blockExitCode,
		Err:      errors.New("exit status 2"),
	}}}, nil)
	hooks := []hookdomain.Hook{{
		Event:   hookdomain.PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if !dec.Block || dec.Reason != "blocked by policy" {
		t.Fatalf("got block=%v reason=%q, want exit-2 block w/ stderr reason", dec.Block, dec.Reason)
	}
}

func TestRunner_AskEscalates(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandAsk, Reason: "review"}}}}, nil)
	hooks := []hookdomain.Hook{{Event: hookdomain.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if dec.Block || !dec.Ask {
		t.Fatalf("got block=%v ask=%v, want ask", dec.Block, dec.Ask)
	}
}

func TestRunner_RewriteArguments(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandAllow, RewriteArguments: `{"path":"safe"}`}}}}, nil)
	hooks := []hookdomain.Hook{{Event: hookdomain.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "write"}})
	if dec.RewriteArguments != `{"path":"safe"}` {
		t.Fatalf("RewriteArguments = %q", dec.RewriteArguments)
	}
}

func TestRunner_NonBlockingErrorProceeds(t *testing.T) {
	var mu sync.Mutex
	var errs []string
	cmds := &commandStub{results: []CommandResult{{
		Stderr:   "boom",
		ExitCode: 3,
		Err:      errors.New("exit status 3"),
	}}}
	r := NewRunner(cmds, func(_ context.Context, _ string, err error) {
		mu.Lock()
		errs = append(errs, err.Error())
		mu.Unlock()
	})
	hooks := []hookdomain.Hook{{Event: hookdomain.PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a non-2 exit must NOT block (broken hook can't brick the agent)")
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "boom") {
		t.Fatalf("onError = %v, want one error mentioning boom", errs)
	}
}

func TestRunner_InvalidVerdictIsObservableAndIgnored(t *testing.T) {
	var got error
	r := NewRunner(
		&commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: "future"}}}},
		func(_ context.Context, _ string, err error) { got = err },
	)
	decision := r.Run(ctxBG(), []hookdomain.Hook{{
		Event: hookdomain.PreToolUse, Command: "hook",
	}}, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if decision.Block || decision.Ask || decision.InjectContext != "" || decision.RewriteArguments != "" {
		t.Fatalf("invalid verdict changed decision: %+v", decision)
	}
	if got == nil || !strings.Contains(got.Error(), "invalid command verdict") {
		t.Fatalf("onError = %v, want invalid verdict error", got)
	}
}

func TestRunner_TimeoutIsNonBlocking(t *testing.T) {
	var got error
	r := NewRunner(&commandStub{results: []CommandResult{{TimedOut: true}}}, func(_ context.Context, _ string, err error) { got = err })
	hooks := []hookdomain.Hook{{Event: hookdomain.PreToolUse, Command: "hook", TimeoutMillis: 40}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a timed-out hook must not block")
	}
	if got == nil || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("onError = %v, want timeout", got)
	}
}

func TestRunner_MatcherGatesByToolName(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandDeny, Reason: "x"}}}}
	r := NewRunner(cmds, nil)
	hooks := []hookdomain.Hook{{Event: hookdomain.PreToolUse, Matcher: "shell", Command: "hook"}}

	denied := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if !denied.Block {
		t.Error("matcher shell should fire for shell")
	}
	passed := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "read"}})
	if passed.Block {
		t.Error("matcher shell must NOT fire for read")
	}
	if cmds.calls() != 1 {
		t.Fatalf("commands called %d times, want only the matching hook", cmds.calls())
	}
}

func TestRunner_FirstBlockWins_ContextConcatenated(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{
		{Decision: CommandDecision{Verdict: CommandDeny, Reason: "first"}},
		{Decision: CommandDecision{Verdict: CommandDeny, Reason: "second"}},
	}}, nil)
	hooks := []hookdomain.Hook{
		{Event: hookdomain.PostToolUse, Inject: "ctx-a"},
		{Event: hookdomain.PostToolUse, Command: "hook"},
		{Event: hookdomain.PostToolUse, Command: "hook"},
		{Event: hookdomain.PostToolUse, Inject: "ctx-b"},
	}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PostToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if dec.Reason != "first" {
		t.Errorf("Reason = %q, want first-block-wins", dec.Reason)
	}
	if dec.InjectContext != "ctx-a\nctx-b" {
		t.Errorf("InjectContext = %q, want both concatenated", dec.InjectContext)
	}
}

func TestRunner_WrongEventDoesNotFire(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Verdict: CommandDeny}}}}
	r := NewRunner(cmds, nil)
	hooks := []hookdomain.Hook{{Event: hookdomain.Stop, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, hookdomain.Input{Event: hookdomain.PreToolUse, Tool: &hookdomain.ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a Stop hook must not fire on PreToolUse")
	}
	if cmds.calls() != 0 {
		t.Fatalf("commands called %d times, want none", cmds.calls())
	}
}
