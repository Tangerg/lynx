package terminal

import (
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (a *app) Handle(event input.Event) bool {
	action := a.action(event)
	a.disarmConfirmation(event, action)
	if action == quitApp {
		a.handleQuit(event)
		return true
	}
	// Oolong modal stacks intentionally consume every key so input cannot leak
	// into covered content. Blocking runtime interactions are the exception at
	// the product-policy layer: their cancel action must resolve the interaction,
	// not disappear into the modal boundary.
	if action == cancelRun && (a.approval != nil || a.question != nil) {
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
	if a.shell.TranscriptFocused() && isTranscriptPaletteEvent(event) {
		a.showCommandPalette()
		return true
	}
	if a.completion.Handle(event) {
		return true
	}
	if a.shell.PromptFocused() && a.handleHistoryAction(action) {
		return true
	}
	if a.shell.PromptFocused() && isSubmitEvent(event) {
		a.submit()
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
	a.history.Add(message)
	a.resetComposer()
	a.completion.Dismiss()
	a.status.note("draft cleared")
	return true
}

func (a *app) handleQuit(event input.Event) {
	if !a.confirmation.Confirm(confirmQuit, time.Now()) {
		key, _ := event.(input.Key)
		a.message("press " + key.String() + " again to quit")
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

func isTranscriptPaletteEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	return ok && key.Down() && key.Code == input.Character && key.Rune == '?' && key.Mods == 0
}

func isPromptTextEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Code != input.Character {
		return false
	}
	return key.Mods == 0 || key.Mods == input.Shift
}

func (a *app) action(event input.Event) keymap.Action {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return ""
	}
	action, _ := a.composer.Editor().Keys.Action(key.Chord())
	return action
}

func (a *app) handleGlobalAction(action keymap.Action) bool {
	if action != cancelRun {
		return false
	}
	a.handleCancelGesture()
	return true
}

func (a *app) handleCancelGesture() {
	if a.approval != nil || a.question != nil {
		a.cancel()
		return
	}
	message, hasDraft, err := a.currentDraft()
	if err != nil {
		a.message(err.Error())
		return
	}
	if hasDraft {
		a.history.Add(message)
		a.resetComposer()
		a.completion.Dismiss()
		a.message("draft cleared; press Ctrl+C again to cancel")
		return
	}
	a.cancel()
}

func (a *app) handleSessionAction(action keymap.Action) bool {
	switch action {
	case commandPalette:
		a.showCommandPalette()
		return true
	case showSessions:
		a.ShowSessions()
		return true
	case cycleMode:
		if !a.shell.PromptFocused() {
			return false
		}
		a.CycleMode()
		return true
	case searchHistory:
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

func isSubmitEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	return ok && key.Down() && key.Code == input.Enter && key.Mods == 0
}
