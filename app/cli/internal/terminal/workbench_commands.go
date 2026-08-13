package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

func (a *app) stashPrompt() error {
	if a.workbench == nil {
		return errors.New("prompt stashes are unavailable")
	}
	message, present, err := a.currentDraft()
	if err != nil {
		return err
	}
	if !present {
		return errors.New("the composer has no prompt to stash")
	}
	// Flush the exact visible value first, serializing this transfer after every
	// autosave. The store can then move that durable draft into the stash
	// collection without racing an older writer that would resurrect it.
	if err := a.saveDraft(message); err != nil {
		a.reportWorkbenchIssue(workbenchDraft, err)
		return fmt.Errorf("save session draft before stashing: %w", err)
	}
	stash, err := a.workbench.StashDraft(a.session.ID, message)
	a.reportWorkbenchIssue(workbenchDraft, err)
	if err != nil {
		return fmt.Errorf("stash session draft: %w", err)
	}
	a.draftState.Reset(a.session.ID, agent.Message{})
	a.restoreComposer(agent.Message{})
	a.message("stashed prompt · " + stash.ID)
	return nil
}

func (a *app) showPromptStashes() {
	stashes := a.workbench.Stashes()
	if len(stashes) == 0 {
		a.message("there are no prompt stashes")
		return
	}
	lines := make([]string, 0, len(stashes))
	for index, stash := range stashes {
		lines = append(lines, fmt.Sprintf("%d. %s · %s · %s", index+1, stash.ID, compactRelativeAge(stash.CreatedAt), promptSummary(stash)))
	}
	a.transcript.Append(&kit.Message{Theme: a.transcript.theme, Speaker: "prompt stashes", Body: strings.Join(lines, "\n")})
}

func (a *app) applyPromptStash(identity string) error {
	if _, present, err := a.currentDraft(); err != nil {
		return err
	} else if present {
		return errors.New("the composer is not empty; stash or clear it before applying another prompt")
	}
	stash, err := a.findPromptStash(identity)
	if err != nil {
		return err
	}
	if err := a.commitDraft(stash.Message); err != nil {
		return fmt.Errorf("apply prompt stash: save session draft: %w", err)
	}
	a.message("applied prompt stash · " + stash.ID)
	return nil
}

func (a *app) deletePromptStash(identity string) error {
	stash, err := a.findPromptStash(identity)
	if err != nil {
		return err
	}
	deleted, err := a.workbench.DeleteStash(stash.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return errors.New("prompt stash disappeared before it could be deleted")
	}
	a.message("deleted prompt stash · " + stash.ID)
	return nil
}

func (a *app) findPromptStash(identity string) (workbench.Stash, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return workbench.Stash{}, errors.New("a prompt stash id or unique prefix is required")
	}
	var matches []workbench.Stash
	for _, stash := range a.workbench.Stashes() {
		if strings.HasPrefix(stash.ID, identity) {
			matches = append(matches, stash)
		}
	}
	switch len(matches) {
	case 0:
		return workbench.Stash{}, fmt.Errorf("prompt stash %q was not found", identity)
	case 1:
		return matches[0], nil
	default:
		return workbench.Stash{}, fmt.Errorf("prompt stash prefix %q is ambiguous", identity)
	}
}

func promptSummary(stash workbench.Stash) string {
	text := strings.TrimSpace(stash.Message.Text)
	if line, _, ok := strings.Cut(text, "\n"); ok {
		text = strings.TrimSpace(line)
	}
	if text == "" && len(stash.Message.Attachments) > 0 {
		text = "@" + stash.Message.Attachments[0].Name
	}
	if text == "" {
		text = "empty prompt"
	}
	if len(stash.Message.Attachments) > 0 {
		text += " · " + countedNoun(len(stash.Message.Attachments), "attachment")
	}
	return text
}
