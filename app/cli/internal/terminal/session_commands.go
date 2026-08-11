package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/attachment"
)

func (a *app) ShowSessions() {
	if a.conversation.Busy() || a.following {
		a.message("finish or cancel the current run before switching sessions")
		return
	}
	a.sessionCenter.Reset()
	a.loadSessionPage("", false)
}

func (a *app) loadMoreSessions() {
	if !a.sessionCenter.HasMore() {
		a.message("all sessions are already loaded")
		return
	}
	a.loadSessionPage(a.sessionCenter.Cursor(), true)
}

func (a *app) loadSessionPage(cursor string, appendPage bool) {
	a.message("loading sessions")
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) (agent.SessionPage, error) {
			page, err := a.runtime.ListSessions(ctx, agent.SessionQuery{Limit: 20, Cursor: cursor})
			if err != nil {
				return agent.SessionPage{}, err
			}
			if err := page.Validate(); err != nil {
				return agent.SessionPage{}, fmt.Errorf("list sessions: %w", err)
			}
			return page, nil
		},
		func(page agent.SessionPage, err error) {
			if err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if err := a.sessionCenter.SetPage(page, appendPage); err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if !a.sessionDialog.Open() {
				a.sessionDialog.Show()
			}
			a.status.note("choose a session")
		},
	)
}

func (a *app) toggleSessionFavorite(session agent.Session) {
	desired := !session.Favorite
	a.updateSessionFromCenter(session.ID, "updating favorite", func(latest agent.Session) agent.UpdateSession {
		return agent.UpdateSession{SessionID: latest.ID, Favorite: &desired, ExpectedRevision: latest.Revision}
	})
}

func (a *app) openSessionRename(session agent.Session) {
	title := displayTitle(session)
	field := &headless.Text{Label: "Session title", Value: headless.Bind(&title), Check: requiredText}
	field.Editor().Clipboard = a.loop.Clipboard()
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		a.sessionRenameDialog.Dismiss()
		trimmed := strings.TrimSpace(title)
		a.updateSessionFromCenter(session.ID, "renaming session", func(latest agent.Session) agent.UpdateSession {
			return agent.UpdateSession{SessionID: latest.ID, Title: &trimmed, ExpectedRevision: latest.Revision}
		})
	}
	form.GaveUp = func() { a.sessionRenameDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.sessionRenameDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Rename session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.sessionRenameDialog.Show()
}

func (a *app) openSessionDelete(session agent.Session) {
	if session.ID == a.session.ID {
		a.message("switch away before deleting the current session")
		return
	}
	decision := "cancel"
	choice := &headless.Select[string]{Label: "Delete " + displayTitle(session) + "?", Value: headless.Bind(&decision), Rows: 2}
	choice.SetOptions([]headless.Option[string]{{Label: "Cancel", Value: "cancel"}, {Label: "Delete permanently", Value: "delete"}})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		a.sessionDeleteDialog.Dismiss()
		if decision == "delete" {
			a.deleteSessionFromCenter(session.ID)
		}
	}
	form.GaveUp = func() { a.sessionDeleteDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.sessionDeleteDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Delete session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 8},
	})
	a.sessionDeleteDialog.Show()
}

func (a *app) updateSessionFromCenter(id, label string, build func(agent.Session) agent.UpdateSession) {
	started := runOperation(a, sessionCenterOperation, false,
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, id)
			if err != nil {
				return agent.Session{}, err
			}
			return a.runtime.UpdateSession(ctx, build(latest.Session))
		},
		func(updated agent.Session, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.sessionCenter.Upsert(updated)
			if updated.ID == a.session.ID {
				a.session = updated
				a.header.SetSession(updated)
				a.setWindowTitle()
			}
			a.message(label + " complete")
		},
	)
	if !started {
		a.message("wait for the current session action to finish")
	}
}

func (a *app) deleteSessionFromCenter(id string) {
	started := runOperation(a, sessionCenterOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, a.runtime.DeleteSession(ctx, agent.DeleteSession{SessionID: id})
		},
		func(_ struct{}, err error) {
			if err != nil {
				a.message("delete session failed: " + err.Error())
				return
			}
			a.sessionCenter.Remove(id)
			a.message("deleted session")
		},
	)
	if !started {
		a.message("wait for the current session action to finish")
	}
}

func (a *app) NewSession() {
	a.startSessionInWorkspace(a.session.Workspace)
}

func (a *app) RenameSession(title string) {
	if a.conversation.Busy() || a.following {
		a.message("finish or cancel the current run before renaming the session")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		a.message("/rename needs a non-empty title")
		return
	}
	sessionID := a.session.ID
	runSessionChange(a, "renaming session",
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.Session{}, err
			}
			return a.runtime.UpdateSession(ctx, agent.UpdateSession{SessionID: sessionID, Title: &title, ExpectedRevision: latest.Session.Revision})
		},
		func(updated agent.Session) error {
			if err := updated.Validate(); err != nil {
				return fmt.Errorf("rename session: %w", err)
			}
			if updated.ID != sessionID {
				return fmt.Errorf("rename session: runtime returned session %s, want %s", updated.ID, sessionID)
			}
			a.session = updated
			a.header.SetSession(updated)
			a.setWindowTitle()
			a.message("renamed session to " + updated.Title)
			return nil
		},
	)
}

func (a *app) ForkSession(title string) {
	source := a.session.ID
	runSessionChange(a, "forking session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{SessionID: source, Title: strings.TrimSpace(title)})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.runtime.GetSession(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) forkSessionFromRun(runID string) {
	source := a.session.ID
	short := shortIdentity(runID)
	runSessionChange(a, "forking session from "+short,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{
				SessionID: source, FromRunID: runID, Title: "Fork from " + short,
			})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.runtime.GetSession(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) switchSession(id string) {
	if id == a.session.ID {
		a.message("already in " + displayTitle(a.session))
		return
	}
	runSessionChange(a, "loading session",
		func(ctx context.Context) (agent.SessionSnapshot, error) { return a.runtime.GetSession(ctx, id) },
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func runSessionChange[T any](a *app, label string, work func(context.Context) (T, error), apply func(T) error) {
	if a.conversation.Busy() || a.following {
		a.message("finish or cancel the current run before changing sessions")
		return
	}
	if a.pendingCancel != nil {
		a.message("wait for runtime cancellation to finish")
		return
	}
	if a.operations.Active(sessionChangeOperation) {
		a.message("wait for the current session change to finish")
		return
	}
	a.operations.Cancel(pickerCatalogOperation)
	a.sessionDialog.Dismiss()
	baseline, _, err := a.currentDraft()
	if err != nil {
		a.message(label + " failed: " + err.Error())
		return
	}
	a.persistDraft()
	a.sessionChangeDraft = &sessionChangeDraft{sessionID: a.session.ID, message: baseline}
	a.message(label)
	if !runOperation(a, sessionChangeOperation, false, work, func(result T, err error) {
		if err != nil {
			a.sessionChangeDraft = nil
			a.message(label + " failed: " + err.Error())
			return
		}
		if err := apply(result); err != nil {
			a.message(label + " failed: " + err.Error())
		}
		a.sessionChangeDraft = nil
	}) {
		a.sessionChangeDraft = nil
		a.message("wait for the current session change to finish")
	}
}

type sessionChangeDraft struct {
	sessionID string
	message   agent.Message
}

type sessionInstallation struct {
	snapshot    agent.SessionSnapshot
	attachments *attachment.Resolver
	projection  sessionProjection
	draft       agent.Message
}

func (a *app) prepareSessionInstallation(snapshot agent.SessionSnapshot) (sessionInstallation, error) {
	attachments, err := attachment.New(snapshot.Session.Workspace)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("session attachments: %w", err)
	}
	projection, err := a.projectSession(snapshot, nil)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("install session: %w", err)
	}
	draft, err := a.prepareDestinationDraft(snapshot.Session)
	if err != nil {
		projection.close()
		return sessionInstallation{}, err
	}
	return sessionInstallation{
		snapshot: snapshot, attachments: attachments, projection: projection, draft: draft,
	}, nil
}

func (a *app) prepareDestinationDraft(session agent.Session) (agent.Message, error) {
	a.persistDraft()
	draft, _, err := a.workbench.Draft(session.ID)
	if err != nil {
		return agent.Message{}, fmt.Errorf("load session draft: %w", err)
	}
	if err := a.workbench.RememberWorkspace(session.Workspace); err != nil {
		return agent.Message{}, fmt.Errorf("remember workspace: %w", err)
	}
	change := a.sessionChangeDraft
	if change == nil {
		return draft, nil
	}
	current, _, err := a.currentDraft()
	if err != nil || current.Equal(change.message) {
		return draft, err
	}
	if err := a.workbench.SaveDraft(change.sessionID, change.message); err != nil {
		return agent.Message{}, fmt.Errorf("restore source session draft: %w", err)
	}
	if err := a.workbench.SaveDraft(session.ID, current); err != nil {
		return agent.Message{}, fmt.Errorf("move draft to destination session: %w", err)
	}
	return current, nil
}

func (installation sessionInstallation) apply(a *app) {
	previousWorkspace := a.session.Workspace
	a.cancelPluginCommands()
	a.operations.CancelScope(sessionOperationScope)
	a.dropStream()
	a.completion.Dismiss()
	previousTranscript := a.transcript
	a.session = installation.snapshot.Session
	a.dispatchingQueueEntry = 0
	a.conversation = installation.projection.conversation
	a.attachments = installation.attachments
	a.transcript = installation.projection.transcript
	a.wireTranscript(installation.projection.transcript)
	a.restoreComposer(installation.draft)
	a.activity.Reset()
	a.status.Reset(a.options)
	a.header.SetSession(installation.snapshot.Session)
	a.header.SetUsage(installation.projection.conversation.Usage())
	a.prompt.SetOptions(a.options)
	a.prompt.SetBusy(installation.projection.conversation.Busy())
	a.shell.SetTranscript(installation.projection.transcript)
	a.syncQueue()
	previousTranscript.Close()
	a.listenForSearch()
	a.setWindowTitle()
	a.restoreActivity(installation.snapshot)
	if a.session.Workspace != previousWorkspace {
		a.followRuntimeChanges()
	}
	if a.conversation.Phase() == agent.ConversationIdle {
		a.message("session · " + displayTitle(installation.snapshot.Session))
		if a.sessionInvalidated {
			a.refreshInvalidatedSession(false)
		}
	}
}

func (a *app) installSnapshot(snapshot agent.SessionSnapshot) error {
	installation, err := a.prepareSessionInstallation(snapshot)
	if err != nil {
		return err
	}
	installation.apply(a)
	return nil
}

func compactRelativeAge(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	duration := time.Since(at)
	switch {
	case duration < time.Minute:
		return "now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}
