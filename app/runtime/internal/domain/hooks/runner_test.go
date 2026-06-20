package hooks

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func ctxBG() context.Context { return context.Background() }

func TestRunner_DeclarativeInject(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{Event: SessionStart, Inject: "remember: use tabs"}}
	dec := r.Run(ctxBG(), hooks, Input{Event: SessionStart})
	if dec.InjectContext != "remember: use tabs" {
		t.Fatalf("InjectContext = %q", dec.InjectContext)
	}
	if dec.Block {
		t.Error("declarative inject should not block")
	}
}

func TestRunner_CommandReceivesEventOnStdin(t *testing.T) {
	r := NewRunner(nil)
	// The hook echoes back an injectContext built from the stdin it received,
	// proving the event JSON reaches the command.
	hooks := []Hook{{
		Event:   UserPromptSubmit,
		Command: `grep -q '"event":"UserPromptSubmit"' && echo '{"injectContext":"saw-event"}'`,
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: UserPromptSubmit, Prompt: "hi"})
	if dec.InjectContext != "saw-event" {
		t.Fatalf("InjectContext = %q — stdin event not delivered?", dec.InjectContext)
	}
}

func TestRunner_StdoutDenyBlocks(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{
		Event:   PreToolUse,
		Command: `echo '{"decision":"deny","reason":"no rm allowed"}'`,
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if !dec.Block || dec.Reason != "no rm allowed" {
		t.Fatalf("got block=%v reason=%q, want deny", dec.Block, dec.Reason)
	}
}

func TestRunner_Exit2Blocks(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{
		Event:   PreToolUse,
		Command: `echo "blocked by policy" >&2; exit 2`,
	}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if !dec.Block || dec.Reason != "blocked by policy" {
		t.Fatalf("got block=%v reason=%q, want exit-2 block w/ stderr reason", dec.Block, dec.Reason)
	}
}

func TestRunner_AskEscalates(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{Event: PreToolUse, Command: `echo '{"decision":"ask","reason":"review"}'`}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if dec.Block || !dec.Ask {
		t.Fatalf("got block=%v ask=%v, want ask", dec.Block, dec.Ask)
	}
}

func TestRunner_RewriteArguments(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{Event: PreToolUse, Command: `echo '{"rewriteArguments":"{\"path\":\"safe\"}"}'`}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "write"}})
	if dec.RewriteArguments != `{"path":"safe"}` {
		t.Fatalf("RewriteArguments = %q", dec.RewriteArguments)
	}
}

func TestRunner_NonBlockingErrorProceeds(t *testing.T) {
	var mu sync.Mutex
	var errs []string
	r := NewRunner(func(_ context.Context, _ string, err error) {
		mu.Lock()
		errs = append(errs, err.Error())
		mu.Unlock()
	})
	// exit 3 (not the block code 2) → broken hook → non-blocking, reported.
	hooks := []Hook{{Event: PreToolUse, Command: `echo "boom" >&2; exit 3`}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if dec.Block {
		t.Error("a non-2 exit must NOT block (broken hook can't brick the agent)")
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "boom") {
		t.Fatalf("onError = %v, want one error mentioning boom", errs)
	}
}

func TestRunner_TimeoutIsNonBlocking(t *testing.T) {
	var got error
	r := NewRunner(func(_ context.Context, _ string, err error) { got = err })
	hooks := []Hook{{Event: PreToolUse, Command: `sleep 5`, TimeoutMs: 40}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if dec.Block {
		t.Error("a timed-out hook must not block")
	}
	if got == nil || !strings.Contains(got.Error(), "timed out") {
		t.Fatalf("onError = %v, want timeout", got)
	}
}

func TestRunner_MatcherGatesByToolName(t *testing.T) {
	r := NewRunner(nil)
	// Only fires for bash; a read tool must not trigger it.
	hooks := []Hook{{Event: PreToolUse, Matcher: "bash", Command: `echo '{"decision":"deny","reason":"x"}'`}}

	denied := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if !denied.Block {
		t.Error("matcher bash should fire for bash")
	}
	passed := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "read"}})
	if passed.Block {
		t.Error("matcher bash must NOT fire for read")
	}
}

func TestRunner_FirstBlockWins_ContextConcatenated(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{
		{Event: PostToolUse, Inject: "ctx-a"},
		{Event: PostToolUse, Command: `echo '{"decision":"deny","reason":"first"}'`},
		{Event: PostToolUse, Command: `echo '{"decision":"deny","reason":"second"}'`},
		{Event: PostToolUse, Inject: "ctx-b"},
	}
	dec := r.Run(ctxBG(), hooks, Input{Event: PostToolUse, Tool: &ToolInput{Name: "bash"}})
	if dec.Reason != "first" {
		t.Errorf("Reason = %q, want first-block-wins", dec.Reason)
	}
	if dec.InjectContext != "ctx-a\nctx-b" {
		t.Errorf("InjectContext = %q, want both concatenated", dec.InjectContext)
	}
}

func TestRunner_WrongEventDoesNotFire(t *testing.T) {
	r := NewRunner(nil)
	hooks := []Hook{{Event: Stop, Command: `echo '{"decision":"deny"}'`}}
	dec := r.Run(ctxBG(), hooks, Input{Event: PreToolUse, Tool: &ToolInput{Name: "bash"}})
	if dec.Block {
		t.Error("a Stop hook must not fire on PreToolUse")
	}
}
