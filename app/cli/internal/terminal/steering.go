package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	steeringoutbox "github.com/Tangerg/lynx/app/cli/internal/steering"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

func (a *app) steerRun(instruction string) error {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return errors.New("/steer needs a non-empty instruction")
	}
	runID, segmentID := a.conversation.RunID(), a.conversation.SegmentID()
	if runID == "" || segmentID == "" || a.conversation.Phase() != agent.ConversationRunning {
		return errors.New("no observed run segment is available to steer")
	}
	draft, _, err := a.currentDraft()
	if err != nil {
		return err
	}
	message := agent.Message{Text: instruction, Attachments: slices.Clone(draft.Attachments)}
	if validateMessageCapabilitiesErr := a.validateMessageCapabilities(message); validateMessageCapabilitiesErr != nil {
		return validateMessageCapabilitiesErr
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		return err
	}
	request := agent.SteerRun{CommandID: commandID, RunID: runID, SegmentID: segmentID, Message: message}
	if validateErr := request.Validate(); validateErr != nil {
		return validateErr
	}

	// Reconstruct the parsed command as a durable ownership precondition. If the
	// process stops before staging, restart restores the command for retry. The
	// following aggregate replacement then transfers it and its attachments into
	// the steer journal atomically.
	sourceDraft := agent.Message{Text: "/steer " + instruction, Attachments: slices.Clone(draft.Attachments)}
	if saveDraftErr := a.saveDraft(sourceDraft); saveDraftErr != nil {
		a.reportWorkbenchIssue(workbenchDraft, saveDraftErr)
		return fmt.Errorf("steer blocked: save command draft: %w", saveDraftErr)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	pending, err := steeringoutbox.Stage(
		a.workbench, a.session.ID, request, sourceDraft, steeringReplayWindow(a.runtimeProfile),
	)
	if err != nil {
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("save steer command journal: %w", err))
		a.restoreComposer(sourceDraft)
		a.draftState.Reset(a.session.ID, sourceDraft)
		return err
	}
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	a.restoreComposer(agent.Message{})
	a.draftState.Reset(a.session.ID, agent.Message{})
	started := a.runSessionSettlement(steerRunOperation, false,
		func(ctx context.Context) (steeringoutbox.Result, error) {
			return steeringoutbox.Deliver(
				ctx, a.runtime, pending, steeringReplayWindow(a.runtimeProfile), runtimeRecoveryBackoff,
			)
		},
		func(result steeringoutbox.Result, deliveryErr error) {
			a.settleSteer(result, deliveryErr, runID)
		},
	)
	if !started {
		recovered, err := a.rejectSteer(pending)
		if err != nil {
			a.restoreComposer(workbenchMergeSteerAttachments(a, message.Attachments))
			return fmt.Errorf("another steer operation is already running; restore attachments: %w", err)
		}
		a.restoreComposer(recovered)
		a.draftState.Reset(a.session.ID, recovered)
		return errors.New("another steer operation is already running")
	}
	return nil
}

func (a *app) settleSteer(result steeringoutbox.Result, deliveryErr error, runID string) {
	switch result.Outcome {
	case mutation.Confirmed:
		if err := a.acknowledgeSteer(result.Pending); err != nil {
			a.message("steer accepted; local settlement pending: " + err.Error())
			return
		}
		a.message("steer accepted for " + shortIdentity(runID))
	case mutation.Rejected:
		recovered, err := a.rejectSteer(result.Pending)
		if err != nil {
			a.restoreComposer(workbenchMergeSteerAttachments(a, result.Pending.Command.Message.Attachments))
			a.message("steer run failed; restored attachments were not saved: " + err.Error())
			return
		}
		a.restoreComposer(recovered)
		a.draftState.Reset(a.session.ID, recovered)
		a.message("steer run failed: " + deliveryErr.Error())
	case mutation.Unknown:
		a.message("steer outcome is unknown; it will be reconciled on restart: " + deliveryErr.Error())
	default:
		a.message("steer settlement returned an invalid outcome")
	}
}

func (a *app) acknowledgeSteer(pending workbench.PendingSteer) error {
	current, _, err := a.currentDraft()
	if err != nil {
		return fmt.Errorf("read composer for steer settlement: %w", err)
	}
	if err := a.saveDraft(current); err != nil {
		a.reportWorkbenchIssue(workbenchDraft, err)
		return fmt.Errorf("save current session draft: %w", err)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	if err := a.workbench.AcknowledgePendingSteer(a.session.ID, pending.Command.CommandID); err != nil {
		a.history.Load(a.workbench.History())
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("settle accepted steer command: %w", err))
		return fmt.Errorf("retire accepted steer command: %w", err)
	}
	a.history.Add(pending.Command.Message)
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	a.reportWorkbenchIssue(workbenchHistory, nil)
	return nil
}

func (a *app) rejectSteer(pending workbench.PendingSteer) (agent.Message, error) {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{}, fmt.Errorf("read composer for attachment recovery: %w", err)
	}
	if saveDraftErr := a.saveDraft(current); saveDraftErr != nil {
		a.reportWorkbenchIssue(workbenchDraft, saveDraftErr)
		return agent.Message{}, fmt.Errorf("save current session draft: %w", saveDraftErr)
	}
	a.reportWorkbenchIssue(workbenchDraft, nil)
	recovered, err := a.workbench.RejectPendingSteer(
		a.session.ID, pending.Command.CommandID, current,
	)
	if err != nil {
		a.reportWorkbenchIssue(workbenchSteerOutbox, fmt.Errorf("settle rejected steer command: %w", err))
		return agent.Message{}, fmt.Errorf("save restored attachments: %w", err)
	}
	a.reportWorkbenchIssue(workbenchSteerOutbox, nil)
	return recovered, nil
}

func workbenchMergeSteerAttachments(a *app, rejected []agent.Attachment) agent.Message {
	current, _, err := a.currentDraft()
	if err != nil {
		return agent.Message{Attachments: slices.Clone(rejected)}
	}
	return workbench.MergeSteerAttachments(current, rejected)
}
