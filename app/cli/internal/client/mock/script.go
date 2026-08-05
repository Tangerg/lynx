// Package mock serves scripted conversations, so the CLI can be built, run and
// tested before a real runtime is wired in.
//
// Everything it returns is obviously synthetic. That is a rule, not an accident:
// a fixture that looks like real output invites someone to trust it. Paths,
// diffs and numbers here are made up and should stay that way.
package mock

import (
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// Step is one scripted emission: wait, then send.
type Step struct {
	Delay time.Duration
	Event client.Event
}

// Script is one run's worth of events, in phases. Prelude plays first; if
// Approval is set the run then parks, and the answer selects Approved or Denied.
// A Script whose Approval has no interrupt id never parks.
type Script struct {
	Prelude  []Step
	Approval client.Approval
	Approved []Step
	Denied   []Step
}

// parks reports whether the script pauses for an answer.
func (s Script) parks() bool { return s.Approval.InterruptID != "" }

const (
	// tick paces one streamed word. Fast enough to feel live, slow enough that
	// a renderer's incremental behaviour is visible to the naked eye.
	tick = 22 * time.Millisecond
	beat = 260 * time.Millisecond
)

// Conversation is the default script: think, run a command, explain, ask to edit
// a file, then either edit it or say why it did not.
func Conversation(prompt string) Script {
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

	var prelude []Step
	prelude = append(prelude, Step{Delay: beat, Event: client.PlanChanged{Items: []client.PlanItem{
		{Title: "Reproduce the flake", Status: client.PlanActive},
		{Title: "Find what the test is really waiting for", Status: client.PlanPending},
		{Title: "Replace the sleep and re-run", Status: client.PlanPending},
	}}})
	prelude = append(prelude, stream("rsn_1", client.BlockReasoning, reasoning)...)
	prelude = append(prelude, tool("tool_1", "shell",
		"go test ./internal/store -run TestCacheExpiry -count=5",
		client.ToolOK, testOutput, "", 3*time.Second+412*time.Millisecond)...)
	prelude = append(prelude, Step{Delay: beat, Event: client.PlanChanged{Items: []client.PlanItem{
		{Title: "Reproduce the flake", Status: client.PlanDone},
		{Title: "Find what the test is really waiting for", Status: client.PlanDone},
		{Title: "Replace the sleep and re-run", Status: client.PlanActive},
	}}})
	prelude = append(prelude, stream("msg_1", client.BlockAssistant, explain)...)

	approved := tool("tool_2", "edit", "internal/store/cache_test.go",
		client.ToolOK, "", diff, 118*time.Millisecond)
	approved = append(approved, tool("tool_3", "shell",
		"go test ./internal/store -run TestCacheExpiry -count=50",
		client.ToolOK, "ok  \tgithub.com/example/store\t2.104s", "", 2*time.Second+104*time.Millisecond)...)
	approved = append(approved, stream("msg_2", client.BlockAssistant, summary)...)
	approved = append(approved, Step{Delay: beat, Event: client.RunFinished{
		Outcome: client.Outcome{Status: client.OutcomeCompleted},
		Usage: client.Usage{
			InputTokens: 18422, OutputTokens: 1163, CachedTokens: 12800,
			CostUSD: 0.0412, Duration: 21 * time.Second,
		},
	}})

	denied := []Step{{Delay: beat, Event: client.BlockCompleted{Block: client.Block{
		ID: "note_1", Kind: client.BlockNotice, Text: "Edit declined — internal/store/cache_test.go left unchanged.",
	}}}}
	denied = append(denied, stream("msg_3", client.BlockAssistant, declined)...)
	denied = append(denied, Step{Delay: beat, Event: client.RunFinished{
		Outcome: client.Outcome{Status: client.OutcomeCompleted},
		Usage: client.Usage{
			InputTokens: 14180, OutputTokens: 742, CachedTokens: 12800,
			CostUSD: 0.0291, Duration: 14 * time.Second,
		},
	}})

	return Script{
		Prelude: prelude,
		Approval: client.Approval{
			InterruptID: "int_1",
			Title:       "edit internal/store/cache_test.go",
			Detail:      "Replace the fixed 50ms sleep with a wait on the janitor's sweep signal.",
			Diff:        diff,
		},
		Approved: approved,
		Denied:   denied,
	}
}

// stream renders one body as a started block, a delta per word, and a completed
// block — the shape a real streaming item takes.
func stream(id string, kind client.BlockKind, text string) []Step {
	steps := []Step{{Delay: beat, Event: client.BlockStarted{Block: client.Block{ID: id, Kind: kind}}}}
	for _, w := range words(text) {
		steps = append(steps, Step{Delay: tick, Event: client.BlockDelta{BlockID: id, Text: w}})
	}
	return append(steps, Step{Event: client.BlockCompleted{Block: client.Block{ID: id, Kind: kind, Text: text}}})
}

// tool renders one call as running, then finished with its result.
func tool(id, name, summary string, status client.ToolStatus, output, diff string, d time.Duration) []Step {
	running := client.Block{ID: id, Kind: client.BlockTool, Tool: &client.ToolCall{
		Name: name, Summary: summary, Status: client.ToolRunning,
	}}
	done := client.Block{ID: id, Kind: client.BlockTool, Tool: &client.ToolCall{
		Name: name, Summary: summary, Status: status, Output: output, Diff: diff, Duration: d,
	}}
	return []Step{
		{Delay: beat, Event: client.BlockStarted{Block: running}},
		{Delay: 3 * beat, Event: client.BlockCompleted{Block: done}},
	}
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

// demoSessions seeds the catalogue with plainly fake history.
func demoSessions() []client.Session {
	now := time.Date(2026, 8, 4, 11, 30, 0, 0, time.UTC)
	return []client.Session{
		{ID: "ses_demo_1", Title: "Flaky cache expiry test", Workspace: "/tmp/demo/store", UpdatedAt: now, Revision: 7},
		{ID: "ses_demo_2", Title: "Rename the shell tool family", Workspace: "/tmp/demo/store", UpdatedAt: now.Add(-90 * time.Minute), Revision: 3},
		{ID: "ses_demo_3", Title: "Draft the release notes", Workspace: "/tmp/demo/docs", UpdatedAt: now.Add(-26 * time.Hour), Revision: 12},
	}
}
