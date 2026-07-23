package hooks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func ctxBG() context.Context { return context.Background() }

type commandStub struct {
	mu       sync.Mutex
	results  []CommandResult
	requests []CommandRequest
}

func (s *commandStub) RunHookCommand(_ context.Context, req CommandRequest) CommandResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if len(s.results) == 0 {
		return CommandResult{}
	}
	out := s.results[0]
	s.results = s.results[1:]
	return out
}

func (s *commandStub) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func TestRunner_DeclarativeInject(t *testing.T) {
	r := NewRunner(nil, nil)
	hooks := []Hook{{Event: SessionStart, Inject: "remember: use tabs"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: SessionStart})
	if dec.InjectContext != "remember: use tabs" {
		t.Fatalf("InjectContext = %q", dec.InjectContext)
	}
	if dec.Block {
		t.Error("declarative inject should not block")
	}
}

func TestRunner_CommandReceivesTypedEvent(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{InjectContext: "saw-event"}}}}
	r := NewRunner(cmds, nil)
	hooks := []Hook{{
		Event:   UserPromptSubmit,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: UserPromptSubmit, Prompt: "hi"})
	if dec.InjectContext != "saw-event" {
		t.Fatalf("InjectContext = %q — stdin event not delivered?", dec.InjectContext)
	}
	if len(cmds.requests) != 1 || cmds.requests[0].Input.Event != UserPromptSubmit || cmds.requests[0].Input.Prompt != "hi" {
		t.Fatalf("request = %+v, want typed prompt event", cmds.requests)
	}
}

func TestRunner_StdoutDenyBlocks(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{Decision: "deny", Reason: "no rm allowed"}}}}, nil)
	hooks := []Hook{{
		Event:   PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
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
	hooks := []Hook{{
		Event:   PreToolUse,
		Command: "hook",
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if !dec.Block || dec.Reason != "blocked by policy" {
		t.Fatalf("got block=%v reason=%q, want exit-2 block w/ stderr reason", dec.Block, dec.Reason)
	}
}

func TestRunner_AskEscalates(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{Decision: "ask", Reason: "review"}}}}, nil)
	hooks := []Hook{{Event: PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if dec.Block || !dec.Ask {
		t.Fatalf("got block=%v ask=%v, want ask", dec.Block, dec.Ask)
	}
}

func TestRunner_RewriteArguments(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{{Decision: CommandDecision{RewriteArguments: `{"path":"safe"}`}}}}, nil)
	hooks := []Hook{{Event: PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "write"}})
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
	hooks := []Hook{{Event: PreToolUse, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a non-2 exit must NOT block (broken hook can't brick the agent)")
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "boom") {
		t.Fatalf("onError = %v, want one error mentioning boom", errs)
	}
}

func TestRunner_TimeoutIsNonBlocking(t *testing.T) {
	var got error
	r := NewRunner(&commandStub{results: []CommandResult{{TimedOut: true}}}, func(_ context.Context, _ string, err error) { got = err })
	hooks := []Hook{{Event: PreToolUse, Command: "hook", TimeoutMs: 40}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a timed-out hook must not block")
	}
	if got == nil || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("onError = %v, want timeout", got)
	}
}

func TestRunner_MatcherGatesByToolName(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Decision: "deny", Reason: "x"}}}}
	r := NewRunner(cmds, nil)
	hooks := []Hook{{Event: PreToolUse, Matcher: "shell", Command: "hook"}}

	denied := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if !denied.Block {
		t.Error("matcher shell should fire for shell")
	}
	passed := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "read"}})
	if passed.Block {
		t.Error("matcher shell must NOT fire for read")
	}
	if cmds.calls() != 1 {
		t.Fatalf("commands called %d times, want only the matching hook", cmds.calls())
	}
}

func TestRunner_FirstBlockWins_ContextConcatenated(t *testing.T) {
	r := NewRunner(&commandStub{results: []CommandResult{
		{Decision: CommandDecision{Decision: "deny", Reason: "first"}},
		{Decision: CommandDecision{Decision: "deny", Reason: "second"}},
	}}, nil)
	hooks := []Hook{
		{Event: PostToolUse, Inject: "ctx-a"},
		{Event: PostToolUse, Command: "hook"},
		{Event: PostToolUse, Command: "hook"},
		{Event: PostToolUse, Inject: "ctx-b"},
	}
	dec := r.Run(ctxBG(), hooks, Input{Event: PostToolUse, Tool: &ToolInput{Name: "shell"}})
	if dec.Reason != "first" {
		t.Errorf("Reason = %q, want first-block-wins", dec.Reason)
	}
	if dec.InjectContext != "ctx-a\nctx-b" {
		t.Errorf("InjectContext = %q, want both concatenated", dec.InjectContext)
	}
}

func TestRunner_WrongEventDoesNotFire(t *testing.T) {
	cmds := &commandStub{results: []CommandResult{{Decision: CommandDecision{Decision: "deny"}}}}
	r := NewRunner(cmds, nil)
	hooks := []Hook{{Event: Stop, Command: "hook"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "shell"}})
	if dec.Block {
		t.Error("a Stop hook must not fire on PreToolUse")
	}
	if cmds.calls() != 0 {
		t.Fatalf("commands called %d times, want none", cmds.calls())
	}
}
