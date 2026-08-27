package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

type attentionPriority uint8

const (
	attentionInformational attentionPriority = iota + 1
	attentionActionRequired
	attentionFailure
)

type attentionSignal struct {
	priority     attentionPriority
	marker       string
	notification string
	supersedes   bool
}

// attentionCenter models whether an event is still unread by the terminal user.
// It is deliberately independent of the transport used for window titles and
// desktop notifications, which keeps focus policy deterministic and testable.
type attentionCenter struct {
	focused bool
	unread  attentionSignal
}

func newAttentionCenter() attentionCenter {
	return attentionCenter{focused: true}
}

// Observe records terminal focus intent. Oolong owns FocusIn in its event loop,
// so any deliberate keyboard, paste, or mouse-down interaction is also treated as
// proof that the user has returned. FocusIn is still accepted for direct hosts.
func (a *attentionCenter) Observe(event input.Event) bool {
	switch event := event.(type) {
	case input.FocusOut:
		a.focused = false
		return false
	case input.FocusIn:
		return a.focus()
	case input.Key:
		if event.Down() {
			return a.focus()
		}
	case input.Paste:
		return a.focus()
	case input.Mouse:
		if event.Action == input.MouseDown {
			return a.focus()
		}
	}
	return false
}

func (a *attentionCenter) focus() bool {
	cleared := a.unread.marker != ""
	a.focused = true
	a.unread = attentionSignal{}
	return cleared
}

func (a *attentionCenter) Raise(signal attentionSignal) bool {
	if a.focused || strings.TrimSpace(signal.marker) == "" {
		return false
	}
	if !signal.supersedes && a.unread.marker != "" && signal.priority < a.unread.priority {
		return false
	}
	a.unread = signal
	return true
}

func (a *attentionCenter) Marker() string { return a.unread.marker }

func (a *app) observeAttention(event input.Event) {
	if a.attention.Observe(event) {
		a.setWindowTitle()
	}
}

func (a *app) raiseAttention(signal attentionSignal) {
	if !a.attention.Raise(signal) {
		return
	}
	a.setWindowTitle()
	if a.settings.UI.Notifications && signal.notification != "" {
		a.loop.Session().Notify(signal.notification)
	}
}

func (a *app) setWindowTitle() {
	title := "lyra — " + displayTitle(a.session)
	if marker := a.attention.Marker(); marker != "" {
		title += " · " + marker
	}
	a.loop.Session().SetTitle(title)
}

func interactionAttention(interactions []agent.Interaction) attentionSignal {
	message := "lyra needs your input"
	if len(interactions) == 1 {
		switch interactions[0].(type) {
		case agent.Approval:
			message = "lyra needs tool approval"
		case agent.Question:
			message = "lyra has a question"
		}
	}
	return attentionSignal{priority: attentionActionRequired, marker: "action required", notification: message}
}

func outcomeAttention(outcome agent.Outcome) attentionSignal {
	notification := outcomeNotification(outcome)
	if notification == "" {
		return attentionSignal{}
	}
	priority, marker := attentionInformational, "run complete"
	switch outcome.Status {
	case agent.OutcomeFailed, agent.OutcomeLost:
		priority, marker = attentionFailure, "run failed"
	case agent.OutcomeCompleted:
	case agent.OutcomeCanceled:
		marker = "run canceled"
	default:
		marker = "run stopped"
	}
	return attentionSignal{priority: priority, marker: marker, notification: notification, supersedes: true}
}

func failureAttention() attentionSignal {
	return attentionSignal{priority: attentionFailure, marker: "run failed", notification: "lyra run failed"}
}
