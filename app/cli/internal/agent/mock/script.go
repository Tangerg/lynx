// Package mock serves scripted conversations, so the CLI can be built, run and
// tested before a real runtime is wired in.
//
// Everything it returns is obviously synthetic. That is a rule, not an accident:
// a fixture that looks like real output invites someone to trust it. Paths,
// diffs and numbers here are made up and should stay that way.
package mock

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// Step is one scripted emission: wait, then send.
type Step struct {
	Delay time.Duration
	Event agent.Event
}

// Script is one run's worth of events. Prelude plays first; Interaction, when
// non-nil, parks the run and Continue maps the typed answer to its continuation.
type Script struct {
	Prelude     []Step
	Interaction agent.Interaction
	Continue    func(agent.Answer) []Step
}

func buildScriptSafely(build func(string) Script, prompt string) (script Script, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("script builder panicked: %v", recovered)
		}
	}()
	script = cloneScript(build(prompt))
	if err := script.validate(); err != nil {
		return Script{}, err
	}
	return script, nil
}

func (s Script) validate() error {
	interrupted := s.interrupts()
	if interrupted {
		if err := agent.ValidateInteraction(s.Interaction); err != nil {
			return err
		}
	} else if s.Continue != nil {
		return errors.New("script without an interaction has a continuation")
	}
	return validateSteps(s.Prelude, !interrupted)
}

func continueSafely(script Script, answer agent.Answer) (steps []Step, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("script continuation panicked: %v", recovered)
		}
	}()
	if script.Continue == nil {
		return nil, errors.New("script interaction has no continuation")
	}
	steps = cloneSteps(script.Continue(agent.CloneAnswer(answer)))
	if err := validateSteps(steps, true); err != nil {
		return nil, err
	}
	return steps, nil
}

func validateSteps(steps []Step, requireFinish bool) error {
	finished := false
	for i, step := range steps {
		if step.Delay < 0 {
			return fmt.Errorf("step %d has a negative delay", i+1)
		}
		if err := agent.ValidateEvent(step.Event); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		switch step.Event.(type) {
		case agent.RunStarted, agent.RunResumed, agent.RunInterrupted:
			return fmt.Errorf("step %d contains a runtime-owned control event", i+1)
		case agent.RunFinished:
			if i != len(steps)-1 {
				return fmt.Errorf("step %d finishes before the script ends", i+1)
			}
			finished = true
		}
	}
	if requireFinish && !finished {
		return errors.New("script path does not finish the run")
	}
	return nil
}

func cloneScript(script Script) Script {
	script.Prelude = cloneSteps(script.Prelude)
	script.Interaction = agent.CloneInteraction(script.Interaction)
	return script
}

func cloneSteps(steps []Step) []Step {
	cloned := slices.Clone(steps)
	for i := range cloned {
		cloned[i].Event = cloneEvent(cloned[i].Event)
	}
	return cloned
}

func (s Script) interrupts() bool { return s.Interaction != nil }

const (
	// tick paces one streamed word. Fast enough to feel live, slow enough that
	// a renderer's incremental behavior is visible to the naked eye.
	tick = 22 * time.Millisecond
	beat = 260 * time.Millisecond
)

// DefaultScript is the default script: think, run a command, explain, ask to edit
// a file, then either edit it or say why it did not.
func DefaultScript(_ string) Script {
	reasoning := "The failure is intermittent, roughly one run in five, which points at timing rather than logic. " +
		"Let me run the test several times and read what the failing run actually reports."

	explain := "`TestCacheExpiry` waits a fixed **50ms** for the janitor goroutine to evict, then asserts the entry is gone.\n\n" +
		"On a loaded machine the janitor has not always woken by then, so the assertion reads a value that is about to be\n" +
		"removed. The sleep is the bug — the test needs to wait for the eviction, not for a duration.\n"

	summary := "Replaced the sleep with a wait on the janitor's eviction signal. Ran the test 50 times: no failures.\n"

	declined := "Left the file alone. If you would rather fix it differently, the janitor already exposes a channel that\n" +
		"closes after each sweep — waiting on that is the smallest change that removes the timing assumption.\n"

	diff := strings.Join([]string{
		"--- a/internal/store/cache_test.go",
		"+++ b/internal/store/cache_test.go",
		"@@ -18,8 +18,8 @@ func TestCacheExpiry(t *testing.T) {",
		" \tc := New(WithTTL(10 * time.Millisecond))",
		" \tc.Set(\"k\", \"v\")",
		" ",
		"-\ttime.Sleep(50 * time.Millisecond)",
		"-",
		"+\t<-c.swept()",
		"+",
		" \tif _, ok := c.Get(\"k\"); ok {",
		" \t\tt.Fatal(\"expected k to have expired\")",
		" \t}",
		"",
	}, "\n")

	testOutput := strings.Join([]string{
		"--- FAIL: TestCacheExpiry (0.05s)",
		"    cache_test.go:24: expected k to have expired",
		"FAIL",
		"FAIL\tgithub.com/example/store\t0.412s",
		"ok  \tgithub.com/example/store\t0.409s",
		"ok  \tgithub.com/example/store\t0.407s",
		"--- FAIL: TestCacheExpiry (0.05s)",
		"    cache_test.go:24: expected k to have expired",
		"FAIL",
		"ok  \tgithub.com/example/store\t0.410s",
	}, "\n")

	prelude := make([]Step, 0, 32)
	prelude = append(prelude, Step{Delay: beat, Event: agent.PlanChanged{Items: []agent.PlanItem{
		{Title: "Reproduce the flake", Status: agent.PlanActive},
		{Title: "Find what the test is really waiting for", Status: agent.PlanPending},
		{Title: "Replace the sleep and re-run", Status: agent.PlanPending},
	}}})
	prelude = append(prelude, stream("rsn_1", agent.BlockReasoning, reasoning)...)
	prelude = append(prelude, tool("tool_1", agent.ToolShell, "shell",
		"go test ./internal/store -run TestCacheExpiry -count=5",
		agent.ToolOK, testOutput, "", 3*time.Second+412*time.Millisecond)...)
	prelude = append(prelude, Step{Delay: beat, Event: agent.PlanChanged{Items: []agent.PlanItem{
		{Title: "Reproduce the flake", Status: agent.PlanDone},
		{Title: "Find what the test is really waiting for", Status: agent.PlanDone},
		{Title: "Replace the sleep and re-run", Status: agent.PlanActive},
	}}})
	prelude = append(prelude, stream("msg_1", agent.BlockAssistant, explain)...)

	approved := tool("tool_2", agent.ToolEdit, "edit", "internal/store/cache_test.go",
		agent.ToolOK, "", diff, 118*time.Millisecond)
	approved = append(approved, tool("tool_3", agent.ToolShell, "shell",
		"go test ./internal/store -run TestCacheExpiry -count=50",
		agent.ToolOK, "ok  \tgithub.com/example/store\t2.104s", "", 2*time.Second+104*time.Millisecond)...)
	approved = append(approved, stream("msg_2", agent.BlockAssistant, summary)...)
	approved = append(approved, Step{Delay: beat, Event: agent.RunFinished{
		Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		Usage: agent.Usage{
			InputTokens: 18422, OutputTokens: 1163, CachedTokens: 12800,
			CostUSD: 0.0412, Duration: 21 * time.Second,
		},
	}})

	denied := make([]Step, 0, 16)
	denied = append(denied, Step{Delay: beat, Event: agent.BlockCompleted{Block: agent.Block{
		ID: "note_1", Kind: agent.BlockNotice, Text: "Edit declined — internal/store/cache_test.go left unchanged.",
	}}})
	denied = append(denied, stream("msg_3", agent.BlockAssistant, declined)...)
	denied = append(denied, Step{Delay: beat, Event: agent.RunFinished{
		Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		Usage: agent.Usage{
			InputTokens: 14180, OutputTokens: 742, CachedTokens: 12800,
			CostUSD: 0.0291, Duration: 14 * time.Second,
		},
	}})

	return Script{
		Prelude: prelude,
		Interaction: agent.Approval{
			InterruptID: "int_1",
			Title:       "edit internal/store/cache_test.go",
			Detail:      "Replace the fixed 50ms sleep with a wait on the janitor's sweep signal.",
			Diff:        diff,
			Risk:        "writes one workspace test file",
			RuleHint:    "edit:internal/store/cache_test.go",
		},
		Continue: func(answer agent.Answer) []Step {
			approval, ok := answer.(agent.ApprovalAnswer)
			if ok && approval.Decision == agent.ApprovalAllow {
				return approved
			}
			return denied
		},
	}
}

// stream renders one body as a started block, a delta per word, and a completed
// block — the shape a real streaming item takes.
func stream(id string, kind agent.BlockKind, text string) []Step {
	steps := []Step{{Delay: beat, Event: agent.BlockStarted{Block: agent.Block{ID: id, Kind: kind}}}}
	for _, w := range words(text) {
		steps = append(steps, Step{Delay: tick, Event: agent.BlockDelta{BlockID: id, Text: w}})
	}
	return append(steps, Step{Event: agent.BlockCompleted{Block: agent.Block{ID: id, Kind: kind, Text: text}}})
}

// tool renders one call as running, then finished with its result.
func tool(id string, kind agent.ToolKind, name, summary string, status agent.ToolStatus, output, patch string, d time.Duration) []Step {
	running := agent.Block{ID: id, Kind: agent.BlockTool, Tool: &agent.ToolCall{
		Kind: kind, Name: name, Summary: summary, Status: agent.ToolRunning,
	}}
	done := agent.Block{ID: id, Kind: agent.BlockTool, Tool: &agent.ToolCall{
		Kind: kind, Name: name, Summary: summary, Status: status, Output: output, Diff: patch, Duration: d,
	}}
	switch kind {
	case agent.ToolShell:
		running.Tool.Command, done.Tool.Command = summary, summary
		code := 0
		if status == agent.ToolError {
			code = 1
		}
		done.Tool.ExitCode = &code
	case agent.ToolEdit, agent.ToolRead:
		running.Tool.Path, done.Tool.Path = summary, summary
	case agent.ToolSearch:
		running.Tool.Query, done.Tool.Query = summary, summary
	case agent.ToolWeb:
		running.Tool.URL, done.Tool.URL = summary, summary
	case agent.ToolUnknown, agent.ToolTask:
		// These kinds do not project a specialized primary field.
	default:
	}
	steps := []Step{{Delay: beat, Event: agent.BlockStarted{Block: running}}}
	for _, chunk := range strings.SplitAfter(output, "\n") {
		if chunk != "" {
			steps = append(steps, Step{Delay: 3 * tick, Event: agent.BlockDelta{BlockID: id, Text: chunk}})
		}
	}
	completionDelay := 3 * beat
	if output != "" {
		completionDelay = beat
	}
	return append(steps, Step{Delay: completionDelay, Event: agent.BlockCompleted{Block: done}})
}

// words splits text so that concatenating the pieces reproduces it exactly.
func words(text string) []string {
	var out []string
	for len(text) > 0 {
		i := strings.IndexByte(text, ' ')
		if i < 0 {
			return append(out, text)
		}
		out = append(out, text[:i+1])
		text = text[i+1:]
	}
	return out
}

// demoSessions seeds the catalog with plainly fake history.
func demoSessions() []agent.Session {
	now := time.Date(2026, 8, 4, 11, 30, 0, 0, time.UTC)
	return []agent.Session{
		{ID: "ses_demo_1", Title: "Flaky cache expiry test", Workspace: "/tmp/demo/store", UpdatedAt: now, Revision: 7},
		{ID: "ses_demo_2", Title: "Rename the shell tool family", Workspace: "/tmp/demo/store", UpdatedAt: now.Add(-90 * time.Minute), Revision: 3},
		{ID: "ses_demo_3", Title: "Draft the release notes", Workspace: "/tmp/demo/docs", UpdatedAt: now.Add(-26 * time.Hour), Revision: 12},
	}
}
