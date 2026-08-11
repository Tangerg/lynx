package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/attachment"
)

func (a *app) ShowSessions() {
	if a.conversation.Busy() || a.following {
		a.message("finish or cancel the current run before switching sessions")
		return
	}
	a.message("loading sessions")
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) (agent.SessionPage, error) {
			page, err := a.runtime.ListSessions(ctx, agent.SessionQuery{Limit: 100})
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
			a.sessionPicker.Reset()
			a.sessionPicker.SetItems(page.Items)
			a.sessionDialog.Show()
			a.status.note("choose a session")
		},
	)
}

func (a *app) NewSession() {
	workspace := a.session.Workspace
	runSessionChange(a, "creating session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			created, err := a.runtime.CreateSession(ctx, agent.CreateSession{Workspace: workspace})
			return agent.SessionSnapshot{Session: created}, err
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
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
			return a.runtime.UpdateSession(ctx, agent.UpdateSession{SessionID: sessionID, Title: title, ExpectedRevision: latest.Session.Revision})
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
			a.loop.Session().SetTitle("lyra — " + displayTitle(updated))
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
	a.message(label)
	if !runOperation(a, sessionChangeOperation, false, work, func(result T, err error) {
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		if err := apply(result); err != nil {
			a.message(label + " failed: " + err.Error())
		}
	}) {
		a.message("wait for the current session change to finish")
	}
}

func (a *app) installSnapshot(snapshot agent.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("install session: %w", err)
	}
	attachments, err := attachment.New(snapshot.Session.Workspace)
	if err != nil {
		return fmt.Errorf("session attachments: %w", err)
	}
	next := agent.NewConversation()
	if err := next.RestoreSnapshot(snapshot); err != nil {
		return fmt.Errorf("install session: %w", err)
	}
	draft, err := a.composerMessage()
	if err != nil {
		return err
	}
	draft.Attachments = nil
	nextTranscript := newTranscriptView(
		a.transcript.theme, a.transcript.glyphs, a.transcript.wheel, a.syntax,
		a.settings.UI.TranscriptRetain, a.transcript.details, a.transcript.clipboard,
	)
	if err := presentSnapshot(nextTranscript, snapshot, a.registry); err != nil {
		nextTranscript.Close()
		return err
	}

	a.dropStream()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	previousTranscript := a.transcript
	a.session = snapshot.Session
	a.dispatchingQueueEntry = 0
	a.conversation = next
	a.attachments = attachments
	a.transcript = nextTranscript
	a.wireTranscript(nextTranscript)
	a.restoreComposer(draft)
	a.activity.Reset()
	a.status.Reset(a.options)
	a.header.SetSession(snapshot.Session)
	a.header.SetUsage(next.Usage())
	a.prompt.SetOptions(a.options)
	a.prompt.SetBusy(next.Busy())
	a.shell.SetTranscript(nextTranscript)
	a.syncQueue()
	previousTranscript.Close()
	a.listenForSearch()
	a.loop.Session().SetTitle("lyra — " + displayTitle(snapshot.Session))
	a.restoreActivity(snapshot)
	if a.conversation.Phase() == agent.ConversationIdle {
		a.message("session · " + displayTitle(snapshot.Session))
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
