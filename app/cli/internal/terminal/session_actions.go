package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/sessionrollback"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
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
	started := a.runOperation(sessionOutputOperation, false,
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
	a.runSessionChange("importing session",
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
	started := a.runOperation(sessionOutputOperation, false,
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
	request    agent.RollbackSession
	settlement sessionrollback.Preview
}

func previewRollback(snapshot agent.SessionSnapshot, request agent.RollbackSession) (rollbackPreview, error) {
	settlement, err := sessionrollback.PreviewSession(snapshot, request)
	return rollbackPreview{request: request, settlement: settlement}, err
}

func (r rollbackPreview) ValidateCommit(snapshot agent.SessionSnapshot) error {
	return r.settlement.ValidateCommit(snapshot)
}

// ValidateApplied proves a history rollback committed when its command result
// was lost behind a post-commit cleanup error. Files-only rollback has no
// observable session projection change and therefore cannot use this proof.
func (r rollbackPreview) ValidateApplied(snapshot agent.SessionSnapshot) error {
	return r.settlement.ValidateApplied(snapshot)
}

func (r rollbackPreview) Description() string {
	boundary := "the empty-session boundary"
	if r.request.ToRunID != "" {
		boundary = shortIdentity(r.request.ToRunID)
	}
	if r.request.Scope == agent.RestoreFiles {
		return fmt.Sprintf("Restore files to %s while keeping chat history?", boundary)
	}
	return fmt.Sprintf("Restore %s to %s and remove %d later runs?", r.request.Scope, boundary, r.settlement.DroppedCount())
}

type rollbackSettlement struct {
	result sessionrollback.Result
	err    error
}

func (a *app) rollbackSession(preview rollbackPreview) {
	a.runSessionChange("rolling back session",
		func(ctx context.Context) (rollbackSettlement, error) {
			result, err := sessionrollback.Execute(
				ctx, a.runtime, a.workbench, preview.settlement,
				rollbackReplayWindow(a.runtimeProfile), runtimeRecoveryBackoff,
			)
			if result.Pending.CommandID == "" {
				return rollbackSettlement{}, err
			}
			return rollbackSettlement{result: result, err: err}, nil
		},
		func(settlement rollbackSettlement) error {
			return a.applyRollbackSettlement(settlement)
		},
	)
}

func (a *app) applyRollbackSettlement(settlement rollbackSettlement) error {
	result := settlement.result
	switch result.Outcome {
	case mutation.Rejected:
		if err := sessionrollback.Reject(a.workbench, result); err != nil {
			a.message("rollback session failed; local intent cleanup failed: " + errors.Join(settlement.err, err).Error())
			return nil
		}
		a.message("rollback session failed: " + settlement.err.Error())
		return nil
	case mutation.Unknown:
		a.message("rollback outcome is unknown; it will be reconciled on restart: " + settlement.err.Error())
		return nil
	case mutation.Confirmed:
	default:
		return errors.New("rollback settlement returned an invalid outcome")
	}

	confirmationErr := sessionrollback.Confirm(a.workbench, result)
	installation, err := a.prepareSessionInstallation(result.Snapshot)
	if err != nil {
		return errors.Join(confirmationErr, err)
	}
	installation.apply(a)
	recovery := installation.rollbackRecovery
	if confirmationErr != nil {
		current, _, draftErr := a.currentDraft()
		if draftErr != nil {
			return errors.Join(confirmationErr, draftErr)
		}
		recovered, merged := workbench.MergeSessionRollbackDraft(current, result.Pending)
		recovery = &workbench.SessionRollbackRecovery{
			Draft: recovered, DroppedCount: len(result.Pending.BeforeRunIDs) - len(result.Pending.AfterRunIDs),
			OpeningImages: result.Pending.OpeningImages, Merged: merged,
		}
		if persistErr := a.recoverDraft(recovered); persistErr != nil {
			label := fmt.Sprintf("rolled back %d runs; restored text was not saved: %v", recovery.DroppedCount, persistErr)
			label = appendRollbackWarnings(label, recovery.OpeningImages, settlement.err, confirmationErr)
			a.message(label)
			return nil
		}
	}
	if recovery == nil {
		return errors.New("confirmed rollback did not produce local recovery state")
	}
	label := fmt.Sprintf("rolled back session · %d runs removed", recovery.DroppedCount)
	if recovery.Merged {
		label += "; restored opening text before the newer draft"
	}
	label = appendRollbackWarnings(label, recovery.OpeningImages, settlement.err, confirmationErr)
	a.message(label)
	return nil
}

func appendRollbackWarnings(label string, images int, cleanupErr, journalErr error) string {
	if images > 0 {
		label += fmt.Sprintf("; %d inline images must be reattached", images)
	}
	if cleanupErr != nil {
		label += "; runtime cleanup warning: " + cleanupErr.Error()
	}
	if journalErr != nil {
		label += "; local recovery journal warning: " + journalErr.Error()
	}
	return label
}

func (a *app) reportSessionRollbackRecovery(recovery workbench.SessionRollbackRecovery) {
	label := fmt.Sprintf("recovered rollback input · %d runs removed", recovery.DroppedCount)
	if recovery.Merged {
		label += "; restored opening text before the newer draft"
	}
	if recovery.OpeningImages > 0 {
		label += fmt.Sprintf("; %d inline images must be reattached", recovery.OpeningImages)
	}
	a.message(label)
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
