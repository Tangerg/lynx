package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

func TestApprovalPaneRoutesInputToTheLastPresentedForm(t *testing.T) {
	transcript := testTranscriptView(t)
	pane := approvalPane{
		theme: transcript.theme, glyphs: transcript.glyphs,
		detail: kit.NewParagraph("", transcript.theme.Text),
	}
	pane.view = kit.Transcript{
		Content: &pane.preview, Scroll: &pane.scroll,
		Theme: transcript.theme, Glyphs: transcript.glyphs,
	}
	form := func(done func()) *kit.Form {
		choice := "only"
		selectField := &headless.Select[string]{Value: headless.Bind(&choice), Rows: 1}
		selectField.SetOptions([]headless.Option[string]{{Label: "Only", Value: "only"}})
		controller := headless.NewForm(selectField)
		controller.Done = done
		return kit.NewForm(kit.FormConfig{Theme: transcript.theme, Glyphs: transcript.glyphs, Controller: controller})
	}
	oldCalls, currentCalls := 0, 0
	pane.form = form(func() { oldCalls++ })
	pane.Focus(true)
	root := headless.NewRoot(&pane)
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	pane.form = form(func() { currentCalls++ })
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 1 || currentCalls != 0 {
		t.Fatalf("stale frame routed to old=%d current=%d forms", oldCalls, currentCalls)
	}

	root.Draw(surface.View())
	root.Handle(input.Key{Code: input.Enter})
	if oldCalls != 1 || currentCalls != 1 {
		t.Fatalf("current frame routed to old=%d current=%d forms", oldCalls, currentCalls)
	}
}

func TestReplacedApprovalFormCannotMutateTheCurrentDraft(t *testing.T) {
	transcript := testTranscriptView(t)
	application := &app{loop: &program.Runtime{}, transcript: transcript}
	application.approvalPane = approvalPane{
		theme: transcript.theme, glyphs: transcript.glyphs,
		detail: kit.NewParagraph("", transcript.theme.Text),
	}
	application.approvalPane.view = kit.Transcript{
		Content: &application.approvalPane.preview, Scroll: &application.approvalPane.scroll,
		Theme: transcript.theme, Glyphs: transcript.glyphs,
	}
	application.interactionReview = &interactionReview{}
	application.approval = &agent.Approval{}
	application.setApprovalForm(approvalAllowOnce)
	retired := application.approvalDraft
	application.approvalPane.Focus(true)
	root := headless.NewRoot(&application.approvalPane)
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	application.interactionReview = &interactionReview{}
	application.approval = &agent.Approval{}
	application.approvalDraft = &approvalDecisionDraft{}
	application.setApprovalForm(approvalAllowOnce)
	root.Handle(input.Key{Code: input.Down})
	if retired.choice != approvalDenyOnce {
		t.Fatalf("retired draft choice = %q, want deny once", retired.choice)
	}
	if current := application.approvalDraft.choice; current != approvalAllowOnce {
		t.Fatalf("current draft choice = %q after stale input, want allow once", current)
	}
}

func TestApprovalChoiceMapsEveryDecisionAndRememberScope(t *testing.T) {
	tests := []struct {
		name     string
		action   approvalAction
		decision agent.ApprovalDecision
		remember agent.RememberScope
	}{
		{name: "allow once", action: approvalAllowOnce, decision: agent.ApprovalApprove, remember: agent.RememberNone},
		{name: "allow session", action: approvalAllowSession, decision: agent.ApprovalApprove, remember: agent.RememberSession},
		{name: "allow project", action: approvalAllowProject, decision: agent.ApprovalApprove, remember: agent.RememberProject},
		{name: "allow global", action: approvalAllowGlobal, decision: agent.ApprovalApprove, remember: agent.RememberGlobal},
		{name: "deny once", action: approvalDenyOnce, decision: agent.ApprovalDeny, remember: agent.RememberNone},
		{name: "deny session", action: approvalDenySession, decision: agent.ApprovalDeny, remember: agent.RememberSession},
		{name: "deny project", action: approvalDenyProject, decision: agent.ApprovalDeny, remember: agent.RememberProject},
		{name: "deny global", action: approvalDenyGlobal, decision: agent.ApprovalDeny, remember: agent.RememberGlobal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer, ok := test.action.Answer()
			if !ok {
				t.Fatalf("approval action %q was not recognized", test.action)
			}
			if answer.Decision != test.decision || answer.Remember != test.remember {
				t.Fatalf("approval action %q = %+v", test.action, answer)
			}
		})
	}
	if answer, ok := approvalEditArgs.Answer(); ok || answer != (agent.ApprovalAnswer{}) {
		t.Fatalf("editor choice projected a runtime answer: %+v, %v", answer, ok)
	}
}

func TestApprovalOptionsKeepOneShotActionsStableAndExposeRememberedDenials(t *testing.T) {
	t.Parallel()
	values := func(options []headless.Option[approvalAction]) []approvalAction {
		return slices.Collect(func(yield func(approvalAction) bool) {
			for _, option := range options {
				if !yield(option.Value) {
					return
				}
			}
		})
	}
	if got, want := values(approvalOptions(false)), []approvalAction{
		approvalAllowOnce, approvalDenyOnce, approvalEditArgs,
	}; !slices.Equal(got, want) {
		t.Fatalf("one-shot approval options = %v, want %v", got, want)
	}
	if got, want := values(approvalOptions(true)), []approvalAction{
		approvalAllowOnce, approvalAllowSession, approvalAllowProject, approvalAllowGlobal,
		approvalDenyOnce, approvalDenySession, approvalDenyProject, approvalDenyGlobal, approvalEditArgs,
	}; !slices.Equal(got, want) {
		t.Fatalf("rememberable approval options = %v, want %v", got, want)
	}
}

func TestApprovalDefaultSelectsEveryConfiguredRememberScope(t *testing.T) {
	tests := []struct {
		scope agent.RememberScope
		want  approvalAction
	}{
		{scope: agent.RememberNone, want: approvalAllowOnce},
		{scope: agent.RememberSession, want: approvalAllowSession},
		{scope: agent.RememberProject, want: approvalAllowProject},
		{scope: agent.RememberGlobal, want: approvalAllowGlobal},
	}
	for _, test := range tests {
		if got := defaultApprovalAction(test.scope); got != test.want {
			t.Errorf("defaultApprovalAction(%q) = %q, want %q", test.scope, got, test.want)
		}
	}
}

func TestApprovalChoiceNormalizationRespectsRememberability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		action       approvalAction
		rememberable bool
		want         approvalAction
	}{
		{name: "rememberable project", action: approvalAllowProject, rememberable: true, want: approvalAllowProject},
		{name: "one shot project", action: approvalAllowProject, want: approvalAllowOnce},
		{name: "one shot global", action: approvalAllowGlobal, want: approvalAllowOnce},
		{name: "one shot deny", action: approvalDenyOnce, want: approvalDenyOnce},
		{name: "one shot remembered deny", action: approvalDenyGlobal, want: approvalAllowOnce},
		{name: "one shot edit", action: approvalEditArgs, want: approvalEditArgs},
		{name: "unknown", action: approvalAction("unexpected"), rememberable: true, want: approvalAllowOnce},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.action.Normalize(test.rememberable); got != test.want {
				t.Fatalf("approval action %q normalized for rememberable=%t to %q, want %q", test.action, test.rememberable, got, test.want)
			}
		})
	}
}
