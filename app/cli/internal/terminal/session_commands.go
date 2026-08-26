package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/session"
	"github.com/Tangerg/lynx/app/cli/internal/sessiondeletion"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
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
	a.runOperation(pickerCatalogOperation, true,
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
			if appendPage && !a.sessionDialog.Open() {
				return
			}
			if err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if err := a.sessionCenter.SetPage(page, appendPage); err != nil {
				a.message("could not load sessions: " + err.Error())
				return
			}
			if !appendPage {
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
	var dialog *kit.Dialog
	form.Done = func() {
		if a.sessionRenameDialog != dialog {
			return
		}
		dialog.Dismiss()
		a.sessionRenameDialog = nil
		trimmed := strings.TrimSpace(title)
		a.updateSessionFromCenter(session.ID, "renaming session", func(latest agent.Session) agent.UpdateSession {
			return agent.UpdateSession{SessionID: latest.ID, Title: &trimmed, ExpectedRevision: latest.Revision}
		})
	}
	form.GaveUp = func() {
		if a.sessionRenameDialog == dialog {
			dialog.Dismiss()
			a.sessionRenameDialog = nil
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Rename session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.sessionRenameDialog = dialog
	dialog.Show()
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
	var dialog *kit.Dialog
	form.Done = func() {
		if a.sessionDeleteDialog != dialog {
			return
		}
		dialog.Dismiss()
		a.sessionDeleteDialog = nil
		if decision == "delete" {
			a.deleteSessionFromCenter(session.ID)
		}
	}
	form.GaveUp = func() {
		if a.sessionDeleteDialog == dialog {
			dialog.Dismiss()
			a.sessionDeleteDialog = nil
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Delete session", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 8},
	})
	a.sessionDeleteDialog = dialog
	dialog.Show()
}

func (a *app) updateSessionFromCenter(id, label string, build func(agent.Session) agent.UpdateSession) {
	started := a.runApplicationOperation(sessionCenterOperation, false,
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, id)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, build(latest.Session))
		},
		func(updated agent.Session, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.sessionCenter.Upsert(updated)
			if updated.ID == a.session.ID {
				a.setActiveSession(updated)
			}
			a.message(label + " complete")
		},
	)
	if !started {
		a.message("wait for the current session action to finish")
	}
}

func (a *app) deleteSessionFromCenter(id string) {
	started := a.runApplicationOperation(sessionCenterOperation, false,
		func(ctx context.Context) (sessiondeletion.Result, error) {
			return sessiondeletion.Execute(
				ctx, a.runtime, a.workbench, id, deletionReplayWindow(a.runtimeProfile), runtimeRecoveryBackoff,
			)
		},
		func(result sessiondeletion.Result, err error) {
			switch result.Outcome {
			case mutation.Rejected:
				if rejectErr := sessiondeletion.Reject(a.workbench, result); rejectErr != nil {
					a.message("delete session failed; local intent cleanup failed: " + errors.Join(err, rejectErr).Error())
					return
				}
				a.message("delete session failed: " + err.Error())
				return
			case mutation.Unknown:
				if result.Request.CommandID == "" {
					a.message("delete session failed: " + err.Error())
					return
				}
				a.message("delete session outcome is unknown; it will be reconciled on restart: " + err.Error())
				return
			case mutation.Confirmed:
			default:
				a.message("delete session returned an invalid settlement outcome")
				return
			}
			if err := sessiondeletion.Confirm(a.workbench, result); err != nil {
				a.message("deleted session; local state cleanup failed: " + err.Error())
				return
			}
			if a.queue != nil {
				a.queue.Clear(result.Request.SessionID)
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
	a.startSessionInWorkspace(a.session.Workspace.Path)
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
	a.runSessionChange("renaming session",
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, agent.UpdateSession{
				SessionID: sessionID, Title: &title, ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated agent.Session) error {
			a.setActiveSession(updated)
			a.message("renamed session to " + updated.Title)
			return nil
		},
	)
}

func (a *app) ForkSession(title string) {
	source := a.session.ID
	a.runSessionChange("forking session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{SessionID: source, Title: strings.TrimSpace(title)})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) forkSessionFromRun(runID string) {
	source := a.session.ID
	short := shortIdentity(runID)
	a.runSessionChange("forking session from "+short,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			forked, err := a.runtime.ForkSession(ctx, agent.ForkSession{
				SessionID: source, FromRunID: runID, Title: "Fork from " + short,
			})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, forked.ID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) switchSession(id string) {
	if id == a.session.ID {
		a.message("already in " + displayTitle(a.session))
		return
	}
	a.runSessionChange("loading session",
		func(ctx context.Context) (agent.SessionSnapshot, error) { return a.runtime.GetSession(ctx, id) },
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func (a *app) runSessionChange[T any](label string, work func(context.Context) (T, error), apply func(T) error) {
	a.runSessionChangeWithDraftDisposition(label, preserveSourceDraft, work, apply)
}

func (a *app) runSessionChangeWithDraftDisposition[T any](
	label string,
	disposition sourceDraftDisposition,
	work func(context.Context) (T, error),
	apply func(T) error,
) {
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
	if a.operations.Active(sessionCenterOperation) {
		a.message("wait for the current session action to finish")
		return
	}
	a.operations.Cancel(pickerCatalogOperation)
	a.sessionDialog.Dismiss()
	baseline, _, err := a.currentDraft()
	if err != nil {
		a.message(label + " failed: " + err.Error())
		return
	}
	if err := a.saveDraft(baseline); err != nil {
		a.reportWorkbenchIssue(workbenchDraft, err)
		a.message("session change blocked: save session draft: " + err.Error())
		return
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	a.sessionDraftTransition = &sessionDraftTransition{
		sourceSessionID: a.session.ID,
		baseline:        baseline,
		disposition:     disposition,
	}
	a.message(label)
	if !a.runOperation(sessionChangeOperation, false, work, func(result T, err error) {
		defer a.settleSessionChange()
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		if err := apply(result); err != nil {
			a.message(label + " failed: " + err.Error())
		}
	}) {
		a.settleSessionChange()
		a.message("wait for the current session change to finish")
	}
}

// cancelSessionChange abandons the pending projection replacement while
// retaining the composer state authored during it. Runtime mutations that
// completed before cancellation remain discoverable through the session list;
// the terminal only withdraws its unfinished local transition.
func (a *app) cancelSessionChange() bool {
	if !a.operations.Active(sessionChangeOperation) {
		return false
	}
	a.operations.Cancel(sessionChangeOperation)
	a.message("session change canceled")
	a.settleSessionChange()
	return true
}

// settleSessionChange closes the terminal-side draft transaction and resumes
// any authoritative refresh that runtime notifications deferred behind it.
func (a *app) settleSessionChange() {
	a.sessionDraftTransition = nil
	if a.sessionInvalidated && a.conversation.Phase() != agent.ConversationRunning &&
		!a.following && a.pendingCancel == nil {
		a.refreshInvalidatedSession(false)
	}
}

type sourceDraftDisposition uint8

const (
	preserveSourceDraft sourceDraftDisposition = iota
	retireSourceDraft
)

// sessionDraftTransition owns the authoring-state boundary while a session
// change is in flight. User-requested navigation preserves the source draft;
// forced replacement transfers it because the source session no longer exists.
type sessionDraftTransition struct {
	sourceSessionID string
	baseline        agent.Message
	disposition     sourceDraftDisposition
}

func (transition sessionDraftTransition) resolve(
	store *workbench.Store,
	destinationSessionID string,
	destinationDraft agent.Message,
	currentDraft agent.Message,
) (agent.Message, error) {
	switch transition.disposition {
	case retireSourceDraft:
		if destinationSessionID == transition.sourceSessionID {
			return agent.Message{}, fmt.Errorf("replacement session reused retired identity %s", destinationSessionID)
		}
		if strings.TrimSpace(currentDraft.Text) == "" && len(currentDraft.Attachments) == 0 {
			return currentDraft, nil
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: transition.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, DestinationBefore: destinationDraft,
			DestinationAfter: currentDraft,
		}); err != nil {
			return agent.Message{}, fmt.Errorf("transfer replacement session draft: %w", err)
		}
		return currentDraft, nil
	case preserveSourceDraft:
		// Mutations such as rollback replace the authoritative projection without
		// changing session identity. prepareDestinationDraft already flushed the
		// current composer into this aggregate, so there is no cross-file transfer.
		if destinationSessionID == transition.sourceSessionID {
			return currentDraft, nil
		}
		if currentDraft.Equal(transition.baseline) {
			return destinationDraft, nil
		}
		if err := store.ApplyDraftTransfer(workbench.DraftTransfer{
			SourceSessionID: transition.sourceSessionID, DestinationSessionID: destinationSessionID,
			SourceBefore: currentDraft, SourceAfter: transition.baseline,
			DestinationBefore: destinationDraft, DestinationAfter: currentDraft,
		}); err != nil {
			return agent.Message{}, fmt.Errorf("transfer session draft: %w", err)
		}
		return currentDraft, nil
	default:
		return agent.Message{}, errors.New("session draft transition has an invalid source disposition")
	}
}

type sessionInstallation struct {
	snapshot         agent.SessionSnapshot
	attachments      *attachment.Resolver
	projection       sessionProjection
	draft            agent.Message
	rollbackRecovery *workbench.SessionRollbackRecovery
}

func (a *app) prepareSessionInstallation(snapshot agent.SessionSnapshot) (sessionInstallation, error) {
	attachments, err := attachment.New(snapshot.Session.Workspace.Path)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("session attachments: %w", err)
	}
	projection, err := a.projectSession(snapshot, nil)
	if err != nil {
		return sessionInstallation{}, fmt.Errorf("install session: %w", err)
	}
	draft, recovery, err := a.prepareDestinationDraft(snapshot.Session)
	if err != nil {
		projection.close()
		return sessionInstallation{}, err
	}
	return sessionInstallation{
		snapshot: snapshot, attachments: attachments, projection: projection, draft: draft,
		rollbackRecovery: recovery,
	}, nil
}

func (a *app) prepareDestinationDraft(
	session agent.Session,
) (agent.Message, *workbench.SessionRollbackRecovery, error) {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{}, nil, err
	}
	if err := a.saveDraft(current); err != nil {
		return agent.Message{}, nil, fmt.Errorf("save source session draft: %w", err)
	}
	if err := a.workbench.ActivateSessionState(session.ID); err != nil {
		return agent.Message{}, nil, fmt.Errorf("activate destination session state: %w", err)
	}
	draft, _, err := a.workbench.Draft(session.ID)
	if err != nil {
		return agent.Message{}, nil, fmt.Errorf("load session draft: %w", err)
	}
	if err := a.workbench.RememberWorkspace(session.Workspace.Path); err != nil {
		return agent.Message{}, nil, fmt.Errorf("remember workspace: %w", err)
	}
	transition := a.sessionDraftTransition
	if transition != nil {
		draft, err = transition.resolve(a.workbench, session.ID, draft, current)
		if err != nil {
			return agent.Message{}, nil, err
		}
	}
	recovery, recovered, err := a.workbench.ConsumeConfirmedSessionRollback(session.ID)
	if err != nil {
		return agent.Message{}, nil, fmt.Errorf("recover session rollback input: %w", err)
	}
	if recovered {
		draft = recovery.Draft
	}
	return draft, optionalRollbackRecovery(recovery, recovered), nil
}

func (a *app) retireSessionState(sessionID string) (int, error) {
	if a.workbench != nil {
		if err := a.workbench.RetireSessionState(sessionID); err != nil {
			return 0, fmt.Errorf("discard session authoring state: %w", err)
		}
	}
	discarded := 0
	if a.queue != nil {
		discarded = a.queue.Clear(sessionID)
	}
	return discarded, nil
}

// readSessionAfterMutation converges the authoritative projection without
// repeating a mutation that may already be durable. Its retry budget is the
// same user-configured transport policy as live run recovery.
func (a *app) readSessionAfterMutation(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	policy := reconnect.New(a.settings.UI.ReconnectAttempts)
	for failures := 0; ; {
		snapshot, err := a.runtime.GetSession(ctx, sessionID)
		if err == nil {
			return snapshot, nil
		}
		failures++
		delay, shouldRetry := policy.Next(failures, err)
		if !shouldRetry {
			return agent.SessionSnapshot{}, err
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return agent.SessionSnapshot{}, err
		}
	}
}

func (installation sessionInstallation) apply(a *app) {
	previousSessionID := a.session.ID
	previousWorkspace := a.session.Workspace
	a.prepareSessionProjectionReplacement(installation.snapshot.Session, installation.projection.conversation)
	a.cancelPluginCommands()
	a.operations.CancelScope(sessionOperationScope)
	a.dropStream()
	a.completion.Dismiss()
	previousTranscript := a.transcript
	a.setActiveSession(installation.snapshot.Session)
	a.queue.ReleaseDispatch(previousSessionID)
	a.openingRunID = ""
	a.conversation = installation.projection.conversation
	a.attachments = installation.attachments
	a.transcript = installation.projection.transcript
	a.wireTranscript(installation.projection.transcript)
	a.restoreComposer(installation.draft)
	a.draftState.Reset(a.session.ID, installation.draft)
	a.activity.Reset()
	a.status.Reset(a.options)
	a.workbenchHealth.enterSession()
	a.status.setProblem(a.workbenchHealth.problem())
	a.header.SetUsage(installation.projection.conversation.Usage())
	a.prompt.SetOptions(a.options)
	a.prompt.SetBusy(installation.projection.conversation.Busy())
	a.shell.SetTranscript(installation.projection.transcript)
	a.syncQueue()
	previousTranscript.Close()
	a.listenForSearch()
	a.setWindowTitle()
	a.restoreActivity(installation.snapshot)
	a.restoreSessionOutbox()
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
	if installation.rollbackRecovery != nil {
		a.reportSessionRollbackRecovery(*installation.rollbackRecovery)
	}
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
