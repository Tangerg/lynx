package views

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/ui/store"
	"github.com/Tangerg/lynx/app/tui/atoms/theme"
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// screen draws a view and returns what it looks like, one string per row.
func screen(w, h int, draw func(grid.View)) []string {
	s := grid.NewSurface(w, h)
	draw(s.View())
	rows := make([]string, 0, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c := s.CellAt(x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				b.WriteByte(' ')
			default:
				b.WriteString(c.Content)
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}

func joined(rows []string) string { return strings.Join(rows, "\n") }

// newChat builds a chat over a state, with the requests it makes recorded.
type recorded struct {
	sent      []string
	answers   []client.Decision
	cancelled int
	quit      int
	// accept decides what Send reports back.
	accept bool
}

func newChat(t *testing.T, s *store.Session) (*Chat, *recorded) {
	t.Helper()
	rec := &recorded{accept: true}
	c := NewChat(theme.Dark())
	c.Send = func(body string) bool {
		rec.sent = append(rec.sent, body)
		return rec.accept
	}
	c.Answer = func(d client.Decision) { rec.answers = append(rec.answers, d) }
	c.Cancel = func() { rec.cancelled++ }
	c.Quit = func() { rec.quit++ }
	c.Update(s)
	return c, rec
}

func typeInto(c *Chat, s string) {
	for _, r := range s {
		c.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func TestChatDrawsAConversation(t *testing.T) {
	s := store.NewSession()
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "u", Kind: client.BlockUser, Text: "why is it flaky?"}})
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "a", Kind: client.BlockAssistant, Text: "A fixed sleep."}})
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "t", Kind: client.BlockTool, Tool: &client.ToolCall{
		Name: "shell", Summary: "go test ./...", Status: client.ToolOK, Output: "ok", Duration: 1200000000,
	}}})
	c, _ := newChat(t, s)

	rows := screen(48, 16, c.Draw)
	all := joined(rows)
	for _, want := range []string{"› why is it flaky?", "A fixed sleep.", "● shell · go test ./...", "│ ok", "✓ 1.2s"} {
		if !strings.Contains(all, want) {
			t.Fatalf("screen is missing %q:\n%s", want, all)
		}
	}
}

func TestChatEnterSendsAndAltEnterBreaksTheLine(t *testing.T) {
	// The right way round for a chat: most messages are one line, so the common case
	// gets the short keystroke.
	c, rec := newChat(t, store.NewSession())
	typeInto(c, "hello")
	c.Handle(input.Key{Code: input.Enter})
	if len(rec.sent) != 1 || rec.sent[0] != "hello" {
		t.Fatalf("sent = %v", rec.sent)
	}
	if got := c.Composer().Text(); got != "" {
		t.Fatalf("field = %q, want it cleared once the message was taken", got)
	}

	typeInto(c, "first")
	c.Handle(input.Key{Code: input.Enter, Mods: input.Alt})
	typeInto(c, "second")
	if got := c.Composer().Text(); got != "first\nsecond" {
		t.Fatalf("field = %q, want two lines", got)
	}
}

func TestChatKeepsAMessageThatWasRefused(t *testing.T) {
	// A composer that cleared itself while the session was busy would lose what the
	// user wrote.
	c, rec := newChat(t, store.NewSession())
	rec.accept = false
	typeInto(c, "held")
	c.Handle(input.Key{Code: input.Enter})
	if got := c.Composer().Text(); got != "held" {
		t.Fatalf("field = %q, want the text kept", got)
	}
}

func TestChatDoesNotSendNothing(t *testing.T) {
	c, rec := newChat(t, store.NewSession())
	c.Handle(input.Key{Code: input.Enter})
	typeInto(c, "   ")
	c.Handle(input.Key{Code: input.Enter})
	if len(rec.sent) != 0 {
		t.Fatalf("sent %v, want nothing", rec.sent)
	}
}

func TestChatStopsARunningRunAndQuitsAnIdleOne(t *testing.T) {
	// The same keystroke a shell uses, meaning the same thing: stop what is happening,
	// or leave if nothing is.
	s := store.NewSession()
	c, rec := newChat(t, s)
	stop := input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}

	c.Handle(stop)
	if rec.quit != 1 || rec.cancelled != 0 {
		t.Fatalf("with nothing running: quit %d, cancelled %d", rec.quit, rec.cancelled)
	}
	s.Apply(client.RunStarted{RunID: "r"})
	c.Update(s)
	c.Handle(stop)
	if rec.cancelled != 1 || rec.quit != 1 {
		t.Fatalf("with a run going: quit %d, cancelled %d", rec.quit, rec.cancelled)
	}
}

func TestChatQuitWorksWhereverTheCursorIs(t *testing.T) {
	c, rec := newChat(t, store.NewSession())
	typeInto(c, "half-written message")
	c.Handle(input.Key{Code: input.Character, Rune: 'd', Mods: input.Ctrl})
	if rec.quit != 1 {
		t.Fatal("the quit chord did not reach the screen")
	}
}

func TestChatTypingIsTypingEvenWhenItSpellsAChord(t *testing.T) {
	// The field is last in line, so a plain "c" is a "c" and not a command.
	c, rec := newChat(t, store.NewSession())
	typeInto(c, "cancel")
	if rec.cancelled != 0 || rec.quit != 0 {
		t.Fatalf("typing triggered commands: cancelled %d, quit %d", rec.cancelled, rec.quit)
	}
	if got := c.Composer().Text(); got != "cancel" {
		t.Fatalf("field = %q", got)
	}
}

func TestChatShowsAnApprovalAndTakesTheAnswer(t *testing.T) {
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{
		InterruptID: "int_1",
		Title:       "edit internal/store/cache_test.go",
		Detail:      "Replace the sleep.",
		Diff:        "--- a\n+++ b\n-\tsleep\n+\twait",
	}})
	c, rec := newChat(t, s)

	all := joined(screen(56, 24, c.Draw))
	for _, want := range []string{"? edit internal/store/cache_test.go", "Replace the sleep.", "Refuse"} {
		if !strings.Contains(all, want) {
			t.Fatalf("screen is missing %q:\n%s", want, all)
		}
	}
	c.Handle(input.Key{Code: input.Character, Rune: 'y'})
	if len(rec.answers) != 1 || !rec.answers[0].Approved {
		t.Fatalf("answers = %+v", rec.answers)
	}
}

func TestAnOpenApprovalTakesTheKeyboard(t *testing.T) {
	// Nothing else on the screen can be acted on until it is answered, and what it is
	// asking about is a change to the user's files.
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	c, rec := newChat(t, s)

	typeInto(c, "hello")
	if got := c.Composer().Text(); got != "" {
		t.Fatalf("field = %q, want the prompt to have taken the keys", got)
	}
	if rec.cancelled != 0 {
		t.Fatal("a screen command fired while a prompt was open")
	}
}

func TestLeavingWorksEvenWithAPromptOpen(t *testing.T) {
	// A prompt takes the keyboard, and one that also swallowed the way out would trap
	// the user in front of a question they may not want to answer.
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	c, rec := newChat(t, s)

	c.Handle(input.Key{Code: input.Character, Rune: 'd', Mods: input.Ctrl})
	if rec.quit != 1 {
		t.Fatal("the prompt swallowed the way out")
	}
	if len(rec.answers) != 0 {
		t.Fatalf("leaving answered the prompt: %+v", rec.answers)
	}
}

func TestTheSafeAnswerIsTheOneSelected(t *testing.T) {
	// An answer given by reflex should be the one that changes nothing.
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	c, rec := newChat(t, s)

	c.Handle(input.Key{Code: input.Enter})
	if len(rec.answers) != 1 {
		t.Fatalf("answers = %+v", rec.answers)
	}
	if rec.answers[0].Approved {
		t.Fatal("pressing Enter without choosing allowed the change")
	}
}

func TestEscapeRefuses(t *testing.T) {
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	c, rec := newChat(t, s)

	c.Handle(input.Key{Code: input.Esc})
	if len(rec.answers) != 1 || rec.answers[0].Approved {
		t.Fatalf("answers = %+v, want a refusal", rec.answers)
	}
}

func TestTheApprovalClosesWhenTheRunMovesOn(t *testing.T) {
	// What is on screen follows from the state, so the two cannot disagree.
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	c, _ := newChat(t, s)
	if !strings.Contains(joined(screen(40, 20, c.Draw)), "? edit") {
		t.Fatal("the prompt is not showing")
	}

	s.Resumed()
	c.Update(s)
	if strings.Contains(joined(screen(40, 20, c.Draw)), "? edit") {
		t.Fatal("the prompt outlived the state that asked for it")
	}
	// And the field has the keyboard back.
	typeInto(c, "next")
	if got := c.Composer().Text(); got != "next" {
		t.Fatalf("field = %q", got)
	}
}

func TestChatShowsThePlanOnlyWhenThereIsOne(t *testing.T) {
	s := store.NewSession()
	c, _ := newChat(t, s)
	if strings.Contains(joined(screen(40, 20, c.Draw)), "Plan") {
		t.Fatal("an empty plan took room from the conversation")
	}
	s.Apply(client.PlanChanged{Items: []client.PlanItem{
		{Title: "Reproduce it", Status: client.PlanDone},
		{Title: "Fix it", Status: client.PlanActive},
	}})
	c.Update(s)
	all := joined(screen(40, 20, c.Draw))
	if !strings.Contains(all, "Plan") || !strings.Contains(all, "Fix it") {
		t.Fatalf("screen is missing the plan:\n%s", all)
	}
	// Collapsed, so only the step in progress is shown and the conversation keeps the
	// rest of the room.
	if strings.Contains(all, "Reproduce it") {
		t.Fatalf("a collapsed plan showed a step that is done:\n%s", all)
	}
}

func TestChatStatusFollowsThePhase(t *testing.T) {
	s := store.NewSession()
	c, _ := newChat(t, s)

	s.Apply(client.RunStarted{RunID: "r"})
	c.Update(s)
	if !strings.Contains(joined(screen(50, 14, c.Draw)), "working") {
		t.Fatal("a running session does not say so")
	}
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "i", Title: "edit"}})
	c.Update(s)
	if !strings.Contains(joined(screen(50, 24, c.Draw)), "waiting for you") {
		t.Fatal("a parked session does not say so")
	}
	s.Apply(client.RunFinished{
		Outcome: client.Outcome{Status: client.OutcomeCompleted},
		Usage:   client.Usage{InputTokens: 1234, OutputTokens: 56, CostUSD: 0.01},
	})
	c.Update(s)
	all := joined(screen(50, 14, c.Draw))
	if !strings.Contains(all, "done") {
		t.Fatalf("a finished session does not say so:\n%s", all)
	}
	if !strings.Contains(all, "1,234") {
		t.Fatalf("the cost is not shown:\n%s", all)
	}
}

func TestChatSaysWhyARunFailed(t *testing.T) {
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeFailed, Error: "provider refused"}})
	c, _ := newChat(t, s)
	if !strings.Contains(joined(screen(60, 14, c.Draw)), "provider refused") {
		t.Fatal("a failure is not reported on screen")
	}
}

func TestChatHintsFollowTheState(t *testing.T) {
	// A hint for stopping a run that is not running is noise.
	s := store.NewSession()
	c, _ := newChat(t, s)
	idle := joined(screen(70, 14, c.Draw))
	if strings.Contains(idle, "stop") {
		t.Fatalf("an idle screen offers to stop something:\n%s", idle)
	}
	if !strings.Contains(idle, "enter send") {
		t.Fatalf("the screen does not say how to send:\n%s", idle)
	}
	s.Apply(client.RunStarted{RunID: "r"})
	c.Update(s)
	if !strings.Contains(joined(screen(70, 14, c.Draw)), "stop") {
		t.Fatal("a running screen does not offer to stop it")
	}
}

func TestChatScrollsTheTranscript(t *testing.T) {
	s := store.NewSession()
	for i := range 40 {
		s.Apply(client.BlockCompleted{Block: client.Block{
			ID: string(rune('a'+i%26)) + string(rune('0'+i/26)), Kind: client.BlockAssistant,
			Text: "line " + strings.Repeat("x", 3) + " " + string(rune('A'+i%26)),
		}})
	}
	c, _ := newChat(t, s)
	before := joined(screen(30, 12, c.Draw))
	c.Handle(input.Mouse{Action: input.WheelUp})
	after := joined(screen(30, 12, c.Draw))
	if before == after {
		t.Fatal("the wheel did not scroll the transcript")
	}
}

func TestChatSurvivesASqueezedTerminal(t *testing.T) {
	// A collapsing layout must look small, not corrupted, and must not panic.
	s := store.NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "i", Title: "edit", Diff: "--- a\n+++ b"}})
	s.Apply(client.PlanChanged{Items: []client.PlanItem{{Title: "step", Status: client.PlanActive}}})
	c, _ := newChat(t, s)
	for _, size := range [][2]int{{0, 0}, {1, 1}, {4, 2}, {10, 3}, {20, 5}, {80, 1}} {
		screen(size[0], size[1], c.Draw)
	}
}
