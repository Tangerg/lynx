package terminal

import (
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (a *app) Handle(event input.Event) bool {
	defer a.persistDraft()
	a.observeAttention(event)
	matched, handled := a.matchConfiguredAction(event)
	if handled {
		return true
	}
	if !matched {
		a.disarmConfirmation(event, "")
	}
	return a.handleUnboundEvent(event)
}

func (a *app) matchConfiguredAction(event input.Event) (matched, handled bool) {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false, false
	}
	keys, matcher := a.applicationKeys, &a.applicationMatcher
	if !a.stack.Empty() {
		// A modal owns every non-global key. Keeping a separate matcher prevents a
		// printable prefix configured for the application from swallowing form input.
		a.applicationMatcher.Clear()
		a.prompt.SetPendingKeySequence("")
		keys, matcher = a.globalKeys, &a.globalMatcher
	} else {
		a.globalMatcher.Clear()
	}
	matched, handled = matcher.Handle(keys, key, func(action keymap.Action) bool {
		a.disarmConfirmation(event, action)
		return a.handleConfiguredAction(event, action)
	})
	if matcher == &a.applicationMatcher {
		a.prompt.SetPendingKeySequence(pendingKeySequenceHint(keys, matcher.Keys()))
	}
	return matched, handled
}

func (a *app) handleConfiguredAction(event input.Event, action keymap.Action) bool {
	if action == quitApp {
		a.handleQuit()
		return true
	}
	// Oolong modal stacks intentionally consume every key so input cannot leak
	// into covered content. Blocking runtime interactions are the exception at
	// the product-policy layer: their cancel action must resolve the interaction,
	// not disappear into the modal boundary.
	if action == cancelRun && (a.approval != nil || a.questionnaire != nil) {
		a.handleCancelGesture()
		return true
	}
	if action == cancelRun && a.queueDialog != nil && a.queueDialog.Open() && !a.queueDrawer.Editing() {
		a.cancel()
		return true
	}
	if !a.stack.Empty() {
		if a.stack.Handle(event) {
			return true
		}
		return a.handleGlobalAction(action)
	}
	if a.handleGlobalAction(action) {
		return true
	}
	if a.handleSessionAction(action) {
		return true
	}
	// Completion owns its navigation and acceptance keys before prompt-level
	// history, submission, or editing actions, matching the visual layer on top.
	if action == sendPrompt && a.exactCommandCompletion() {
		a.completion.Dismiss()
	} else if a.completion.Handle(event) {
		return true
	}
	if a.shell.PromptFocused() && a.handleHistoryAction(action) {
		return true
	}
	if !a.shell.PromptFocused() {
		return false
	}
	switch action {
	case sendPrompt:
		a.submit()
		return true
	case insertNewline:
		if a.composer.Editor().Do(action) {
			a.refreshCompletion()
			return true
		}
	}
	return false
}

func (a *app) handleUnboundEvent(event input.Event) bool {
	if !a.stack.Empty() {
		return a.stack.Handle(event)
	}
	if a.shell.TranscriptFocused() {
		switch a.transcript.action(event) {
		case commandPalette:
			a.showCommandPalette()
			return true
		case openReader:
			a.OpenReader()
			return true
		}
	}
	if a.completion.Handle(event) {
		return true
	}
	handled := a.stack.Handle(event)
	if !handled && isEscapeEvent(event) {
		return a.handleEscape()
	}
	if !handled && a.shell.TranscriptFocused() && isPromptTextEvent(event) {
		a.shell.FocusPrompt()
		handled = a.stack.Handle(event)
	}
	if handled {
		a.refreshCompletion()
	}
	return handled
}

func (a *app) disarmConfirmation(event input.Event, action keymap.Action) {
	switch event := event.(type) {
	case input.Key:
		if !event.Down() {
			return
		}
		if a.confirmation.Armed(confirmClearDraft) && event.Code == input.Esc {
			return
		}
		if a.confirmation.Armed(confirmQuit) && action == quitApp {
			return
		}
	case input.Paste:
	case input.Mouse:
		if event.Action != input.MouseDown {
			return
		}
	default:
		return
	}
	a.confirmation.Reset()
}

func isEscapeEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	return ok && key.Down() && key.Code == input.Esc
}

func (a *app) handleEscape() bool {
	if a.conversation.Busy() || a.following || a.pendingCancel != nil {
		a.confirmation.Reset()
		a.cancel()
		return true
	}
	message, hasDraft, err := a.currentDraft()
	if err != nil {
		a.message(err.Error())
		return true
	}
	if !hasDraft || !a.shell.PromptFocused() {
		a.confirmation.Reset()
		return false
	}
	if !a.confirmation.Confirm(confirmClearDraft, time.Now()) {
		a.status.note("press Esc again to clear the draft")
		return true
	}
	a.rememberPrompt(message)
	a.resetComposer()
	a.completion.Dismiss()
	a.status.note("draft cleared")
	return true
}

func (a *app) handleQuit() {
	if !a.confirmation.Confirm(confirmQuit, time.Now()) {
		a.message("repeat " + formatKeyBindings(a.applicationKeys, quitApp, " or ") + " to quit")
		return
	}
	a.loop.Quit()
}

func (a *app) currentDraft() (agent.Message, bool, error) {
	editor := a.composer.Editor()
	if editor.Empty() && len(editor.Elements()) == 0 {
		return agent.Message{}, false, nil
	}
	message, err := a.composerMessage()
	if err != nil {
		return agent.Message{}, false, err
	}
	return message, true, nil
}

func isPromptTextEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Code != input.Character {
		return false
	}
	return key.Mods == 0 || key.Mods == input.Shift
}

func (a *app) handleGlobalAction(action keymap.Action) bool {
	if action != cancelRun {
		return false
	}
	a.handleCancelGesture()
	return true
}

func (a *app) handleCancelGesture() {
	if a.approval != nil || a.questionnaire != nil {
		a.cancel()
		return
	}
	message, hasDraft, err := a.currentDraft()
	if err != nil {
		a.message(err.Error())
		return
	}
	if hasDraft {
		a.rememberPrompt(message)
		a.resetComposer()
		a.completion.Dismiss()
		a.message("draft cleared; repeat " + formatKeyBindings(a.applicationKeys, cancelRun, " or ") + " to cancel")
		return
	}
	a.cancel()
}

func (a *app) handleSessionAction(action keymap.Action) bool {
	switch action {
	case commandPalette:
		a.showCommandPalette()
		return true
	case showShortcuts:
		a.showShortcutDialog()
		return true
	case showSessions:
		a.ShowSessions()
		return true
	case chooseModel, editPrompt:
		return a.handlePromptAction(action)
	case searchTranscript:
		a.showSearchDialog()
		return true
	case manageQueue:
		a.ShowQueue()
		return true
	case toggleDetails:
		a.ToggleToolDetails()
		return true
	case nextMatch:
		a.NextMatch()
		return true
	case previousMatch:
		a.PreviousMatch()
		return true
	case scrollPageUp, scrollPageDown, scrollTop, scrollBottom:
		if a.completion.Open() {
			return false
		}
		return a.transcript.Scroll(action)
	default:
		return false
	}
}

func (a *app) handlePromptAction(action keymap.Action) bool {
	if !a.shell.PromptFocused() {
		return false
	}
	if action == chooseModel {
		a.ChooseModel()
		return true
	}
	if err := a.editPromptExternally(); err != nil {
		a.message(err.Error())
	}
	return true
}

func (a *app) handleHistoryAction(action keymap.Action) bool {
	switch action {
	case historyPrevious:
		if !a.recallPrevious() {
			a.composer.Editor().MoveUp()
		}
		return true
	case historyNext:
		if !a.recallNext() {
			a.composer.Editor().MoveDown()
		}
		return true
	default:
		return false
	}
}
