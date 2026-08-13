package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
)

type sessionImport struct {
	path     string
	artifact sessiontransfer.Document
}

func (a *app) prepareSessionImport(path string) error {
	if a.transfers == nil {
		return errors.New("this runtime composition has no session transfer service")
	}
	if err := a.requireRuntimeFeature(runtimeprofile.FeatureSessionExport); err != nil {
		return err
	}
	workspace := a.session.Workspace.Path
	a.message("reading session artifact")
	started := runOperation(a, sessionOutputOperation, false,
		func(context.Context) (sessionImport, error) {
			artifact, err := a.artifacts.Load(workspace, path)
			return sessionImport{path: path, artifact: artifact}, err
		},
		func(prepared sessionImport, err error) {
			if err != nil {
				a.message("import session failed: " + err.Error())
				return
			}
			a.confirmAction(
				"Import session",
				"Import "+prepared.path+"? An existing session with the artifact ID will be replaced.",
				"Import and replace",
				func() { a.importSession(prepared.artifact) },
			)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func (a *app) importSession(artifact sessiontransfer.Document) {
	runSessionChange(a, "importing session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			imported, err := a.transfers.ImportSession(ctx, sessiontransfer.ImportRequest{Artifact: artifact})
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.readSessionAfterMutation(ctx, imported.ID)
		},
		func(snapshot agent.SessionSnapshot) error {
			if err := a.installSnapshot(snapshot); err != nil {
				return err
			}
			a.message("imported session · " + displayTitle(snapshot.Session))
			return nil
		},
	)
}

func (a *app) prepareSessionRollback(argument string) error {
	request, err := parseRollbackArgument(a.session.ID, argument)
	if err != nil {
		return err
	}
	if request.Scope != agent.RestoreHistory {
		if err := a.requireRuntimeFeature(runtimeprofile.FeatureCheckpoints); err != nil {
			return err
		}
	}
	a.message("previewing rollback")
	started := runOperation(a, sessionOutputOperation, false,
		func(ctx context.Context) (rollbackPreview, error) {
			snapshot, err := a.runtime.GetSession(ctx, request.SessionID)
			if err != nil {
				return rollbackPreview{}, err
			}
			return previewRollback(snapshot, request)
		},
		func(preview rollbackPreview, err error) {
			if err != nil {
				a.message("preview rollback failed: " + err.Error())
				return
			}
			a.confirmAction(
				"Rollback session",
				preview.Description()+" This cannot be undone.",
				"Rollback permanently",
				func() { a.rollbackSession(preview) },
			)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

type rollbackPreview struct {
	request         agent.RollbackSession
	sessionRevision uint64
	keptIDs         []string
	droppedIDs      []string
	openingText     string
	openingImages   int
}

func previewRollback(snapshot agent.SessionSnapshot, request agent.RollbackSession) (rollbackPreview, error) {
	if err := snapshot.Validate(); err != nil {
		return rollbackPreview{}, fmt.Errorf("preview rollback: %w", err)
	}
	if snapshot.Session.ID != request.SessionID {
		return rollbackPreview{}, errors.New("preview rollback: runtime returned another session")
	}
	boundary := -1
	if request.ToRunID != "" {
		boundary = slices.IndexFunc(snapshot.Runs, func(run agent.Run) bool { return run.ID == request.ToRunID })
		if boundary < 0 {
			return rollbackPreview{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, request.ToRunID)
		}
	}
	allIDs := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		allIDs[index] = run.ID
	}
	preview := rollbackPreview{
		request: request, sessionRevision: snapshot.Session.Revision,
		keptIDs: slices.Clone(allIDs[:boundary+1]),
	}
	if request.Scope != agent.RestoreFiles {
		preview.droppedIDs = slices.Clone(allIDs[boundary+1:])
		preview.openingText, preview.openingImages = rollbackOpeningInput(snapshot.Transcript, preview.droppedIDs)
	}
	return preview, nil
}

func rollbackOpeningInput(transcript []agent.Block, droppedIDs []string) (string, int) {
	for _, runID := range droppedIDs {
		for _, block := range transcript {
			if block.RunID != runID || block.Kind != agent.BlockUser {
				continue
			}
			images := 0
			for _, attachment := range block.Attachments {
				if attachment.Kind == agent.AttachmentImage {
					images++
				}
			}
			if strings.TrimSpace(block.Text) != "" || images > 0 {
				return block.Text, images
			}
		}
	}
	return "", 0
}

func (preview rollbackPreview) ValidateCommit(snapshot agent.SessionSnapshot) error {
	current, err := previewRollback(snapshot, preview.request)
	if err != nil {
		return err
	}
	if current.sessionRevision != preview.sessionRevision || !slices.Equal(current.droppedIDs, preview.droppedIDs) {
		return errors.New("session changed after the rollback preview; review the action again")
	}
	return nil
}

// ValidateApplied proves a history rollback committed when its command result
// was lost behind a post-commit cleanup error. Files-only rollback has no
// observable session projection change and therefore cannot use this proof.
func (preview rollbackPreview) ValidateApplied(snapshot agent.SessionSnapshot) error {
	if preview.request.Scope == agent.RestoreFiles {
		return errors.New("files-only rollback has no authoritative session outcome")
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("read rollback outcome: %w", err)
	}
	if snapshot.Session.ID != preview.request.SessionID {
		return errors.New("read rollback outcome: runtime returned another session")
	}
	remaining := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		remaining[index] = run.ID
	}
	if len(preview.droppedIDs) == 0 || snapshot.Session.Revision <= preview.sessionRevision ||
		!slices.Equal(remaining, preview.keptIDs) {
		return errors.New("authoritative session does not prove the rollback committed")
	}
	return nil
}

func (preview rollbackPreview) Description() string {
	boundary := "the empty-session boundary"
	if preview.request.ToRunID != "" {
		boundary = shortIdentity(preview.request.ToRunID)
	}
	if preview.request.Scope == agent.RestoreFiles {
		return fmt.Sprintf("Restore files to %s while keeping chat history?", boundary)
	}
	return fmt.Sprintf("Restore %s to %s and remove %d later root runs?", preview.request.Scope, boundary, len(preview.droppedIDs))
}

type rollbackInstallation struct {
	result        agent.RollbackResult
	snapshot      agent.SessionSnapshot
	droppedCount  int
	openingText   string
	openingImages int
	cleanupErr    error
}

func (a *app) rollbackSession(preview rollbackPreview) {
	request := preview.request
	commandID, err := agent.NewCommandID()
	if err != nil {
		a.message("rollback session failed: create command identity: " + err.Error())
		return
	}
	request.CommandID = commandID
	runSessionChange(a, "rolling back session",
		func(ctx context.Context) (rollbackInstallation, error) {
			latest, err := a.runtime.GetSession(ctx, request.SessionID)
			if err != nil {
				return rollbackInstallation{}, err
			}
			if err := preview.ValidateCommit(latest); err != nil {
				return rollbackInstallation{}, err
			}
			result, rollbackErr := mutation.Confirm(ctx, runtimeRecoveryBackoff, func(ctx context.Context) (agent.RollbackResult, error) {
				return a.runtime.RollbackSession(ctx, request)
			})
			snapshot, err := a.readSessionAfterMutation(ctx, request.SessionID)
			if err != nil {
				return rollbackInstallation{}, errors.Join(rollbackErr, err)
			}
			if rollbackErr != nil {
				if outcomeErr := preview.ValidateApplied(snapshot); outcomeErr != nil {
					return rollbackInstallation{}, errors.Join(rollbackErr, outcomeErr)
				}
				return rollbackInstallation{
					result: agent.RollbackResult{Session: snapshot.Session}, snapshot: snapshot,
					droppedCount: len(preview.droppedIDs), openingText: preview.openingText,
					openingImages: preview.openingImages, cleanupErr: rollbackErr,
				}, nil
			}
			installation := rollbackInstallation{
				result: result, snapshot: snapshot, droppedCount: len(result.Dropped),
			}
			if input, ok := result.FirstOpeningInput(); ok {
				installation.openingText, installation.openingImages = input.OpeningText()
			}
			return installation, nil
		},
		func(installation rollbackInstallation) error {
			if installation.result.Session.ID != installation.snapshot.Session.ID {
				return errors.New("rollback result and authoritative session disagree")
			}
			if err := a.installSnapshot(installation.snapshot); err != nil {
				return err
			}
			if strings.TrimSpace(installation.openingText) != "" {
				if err := a.recoverDraft(agent.Message{Text: installation.openingText}); err != nil {
					label := fmt.Sprintf("rolled back %d runs; restored text was not saved: %v", installation.droppedCount, err)
					if installation.openingImages > 0 {
						label += fmt.Sprintf("; %d inline images must be reattached", installation.openingImages)
					}
					if installation.cleanupErr != nil {
						label += "; runtime cleanup warning: " + installation.cleanupErr.Error()
					}
					a.message(label)
					return nil
				}
			}
			if installation.openingImages > 0 {
				label := fmt.Sprintf("rolled back %d runs; restored text, but %d inline images must be reattached", installation.droppedCount, installation.openingImages)
				if installation.cleanupErr != nil {
					label += "; runtime cleanup warning: " + installation.cleanupErr.Error()
				}
				a.message(label)
				return nil
			}
			label := fmt.Sprintf("rolled back session · %d runs removed", installation.droppedCount)
			if installation.cleanupErr != nil {
				label += "; runtime cleanup warning: " + installation.cleanupErr.Error()
			}
			a.message(label)
			return nil
		},
	)
}

func parseRollbackArgument(sessionID, argument string) (agent.RollbackSession, error) {
	fields := strings.Fields(argument)
	if len(fields) == 0 || len(fields) > 2 {
		return agent.RollbackSession{}, errors.New("usage: /rollback <run-id|all> [history|files|both]")
	}
	boundary := fields[0]
	if strings.EqualFold(boundary, "all") {
		boundary = ""
	}
	scope := agent.RestoreHistory
	if len(fields) == 2 {
		scope = agent.RestoreScope(strings.ToLower(fields[1]))
	}
	request := agent.RollbackSession{SessionID: sessionID, ToRunID: boundary, Scope: scope}
	if err := request.Validate(); err != nil {
		return agent.RollbackSession{}, err
	}
	return request, nil
}

func (a *app) confirmAction(title, question, action string, confirm func()) {
	a.dismissConfirmation()
	generation := a.sessionContext
	decision := "cancel"
	choice := &headless.Select[string]{Label: question, Value: headless.Bind(&decision), Rows: 2}
	choice.SetOptions([]headless.Option[string]{
		{Label: "Cancel", Value: "cancel"},
		{Label: action, Value: "confirm"},
	})
	form := headless.NewForm(choice)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	dismiss := func() {
		if a.confirmationDialog == dialog {
			dialog.Dismiss()
			a.confirmationDialog = nil
		}
	}
	form.Done = func() {
		current := a.confirmationDialog == dialog
		dismiss()
		if current && decision == "confirm" && generation == a.sessionContext {
			confirm()
		}
	}
	form.GaveUp = dismiss
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body, Where: layout.Placement{Width: 78, Height: 9},
	})
	a.confirmationDialog = dialog
	dialog.Show()
}

func (a *app) dismissConfirmation() {
	if a.confirmationDialog != nil {
		a.confirmationDialog.Dismiss()
		a.confirmationDialog = nil
	}
}
