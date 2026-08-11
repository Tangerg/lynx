package terminal

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

func TestAttentionCenterRetainsTheMostImportantUnreadSignalUntilUserInput(t *testing.T) {
	center := newAttentionCenter()
	if center.Raise(attentionSignal{priority: attentionFailure, marker: "failed"}) {
		t.Fatal("a focused terminal must not accumulate unread attention")
	}
	center.Observe(input.FocusOut{})
	if !center.Raise(attentionSignal{priority: attentionFailure, marker: "failed"}) {
		t.Fatal("an unfocused terminal should retain attention")
	}
	if center.Raise(attentionSignal{priority: attentionInformational, marker: "complete"}) {
		t.Fatal("a completion must not replace a more important failure")
	}
	if got := center.Marker(); got != "failed" {
		t.Fatalf("marker = %q, want failed", got)
	}
	if !center.Observe(input.Key{Code: input.Character, Rune: 'x'}) {
		t.Fatal("deliberate user input should clear unread attention")
	}
	if got := center.Marker(); got != "" {
		t.Fatalf("marker after focus = %q", got)
	}
}

func TestTerminalOutcomeSupersedesAnObsoleteActionRequiredMarker(t *testing.T) {
	center := newAttentionCenter()
	center.Observe(input.FocusOut{})
	center.Raise(attentionSignal{priority: attentionActionRequired, marker: "action required"})
	center.Raise(attentionSignal{priority: attentionInformational, marker: "run canceled", supersedes: true})
	if got := center.Marker(); got != "run canceled" {
		t.Fatalf("terminal outcome marker = %q", got)
	}
}

type attentionTestHost struct {
	*programtest.Host
	titles        chan string
	notifications chan string
}

func newAttentionTestHost(t *testing.T) *attentionTestHost {
	return &attentionTestHost{
		Host:          programtest.New(t, 96, 28),
		titles:        make(chan string, 32),
		notifications: make(chan string, 32),
	}
}

func (h *attentionTestHost) SetTitle(title string) { h.titles <- title }
func (h *attentionTestHost) Notify(text string)    { h.notifications <- text }

func runUIWithAttentionHost(t *testing.T, backend agent.Runtime) (*attentionTestHost, func()) {
	t.Helper()
	host := newAttentionTestHost(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: backend, Workspace: "/tmp/lyra-attention-test", Host: host})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestUnfocusedRunCompletionNotifiesAndMarksTheTitleUntilInput(t *testing.T) {
	backend := mock.New()
	backend.Script = stableCompletedScript
	host, stop := runUIWithAttentionHost(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("finish in the background")
	host.Press(input.Enter)
	host.Send(input.FocusOut{})

	waitChannelContains(t, host.notifications, "run completed")
	waitChannelContains(t, host.titles, "run complete")
	host.Type("x")
	title := waitChannelContains(t, host.titles, "lyra — Untitled session")
	if strings.Contains(title, "run complete") {
		t.Fatalf("focused title still carries unread marker: %q", title)
	}
	stop()
}

func TestUnfocusedApprovalRequestsAttention(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Prelude: []mock.Step{{Delay: 50 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{
				ID: "thinking", Kind: agent.BlockReasoning, Text: "checking permissions",
			}}}},
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval", Title: "Write config", Tool: &agent.ToolCall{
					Kind: agent.ToolEdit, Name: "edit", Path: "config.json", Status: agent.ToolRunning,
				},
			}},
			Continue: func([]agent.InterruptAnswer) []mock.Step {
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWithAttentionHost(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("update config")
	host.Press(input.Enter)
	host.Send(input.FocusOut{})

	waitChannelContains(t, host.notifications, "tool approval")
	waitChannelContains(t, host.titles, "action required")
	stop()
}

func waitChannelContains(t *testing.T, values <-chan string, fragment string) string {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case value := <-values:
			if strings.Contains(value, fragment) {
				return value
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q", fragment)
		}
	}
}
