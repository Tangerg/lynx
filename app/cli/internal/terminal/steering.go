package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
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
	if err := a.validateMessageCapabilities(message); err != nil {
		return err
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		return err
	}
	request := agent.SteerRun{CommandID: commandID, RunID: runID, SegmentID: segmentID, Message: message}
	if err := request.Validate(); err != nil {
		return err
	}

	// The command text has already been removed. Retire the staged attachments
	// durably before the runtime can accept them; a refused steer recovers them
	// without overwriting text entered while the call is in flight.
	if err := a.commitDraft(agent.Message{}); err != nil {
		return fmt.Errorf("steer blocked: clear session draft: %w", err)
	}
	started := runOperation(a, steerRunOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return mutation.Confirm(ctx, runtimeRecoveryBackoff, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, a.runtime.SteerRun(ctx, request)
			})
		},
		func(_ struct{}, err error) {
			if err != nil {
				if restoreErr := a.restoreSteerAttachments(message.Attachments); restoreErr != nil {
					a.message("steer run failed; restored attachments were not saved: " + restoreErr.Error())
					return
				}
				a.message("steer run failed: " + err.Error())
				return
			}
			if err := a.rememberPrompt(message); err != nil {
				a.message("steer accepted; save prompt history failed: " + err.Error())
				return
			}
			a.message("steer accepted for " + shortIdentity(runID))
		},
	)
	if !started {
		if err := a.restoreSteerAttachments(message.Attachments); err != nil {
			return fmt.Errorf("another steer operation is already running; restore attachments: %w", err)
		}
		return errors.New("another steer operation is already running")
	}
	return nil
}

func (a *app) restoreSteerAttachments(rejected []agent.Attachment) error {
	if len(rejected) == 0 {
		return nil
	}
	current, _, err := a.currentDraft()
	if err != nil {
		return fmt.Errorf("read composer for attachment recovery: %w", err)
	}
	seen := make(map[string]struct{}, len(current.Attachments)+len(rejected))
	for _, attachment := range current.Attachments {
		seen[attachment.ID] = struct{}{}
	}
	for _, attachment := range rejected {
		if _, duplicate := seen[attachment.ID]; duplicate {
			continue
		}
		current.Attachments = append(current.Attachments, attachment)
		seen[attachment.ID] = struct{}{}
	}
	if err := a.recoverDraft(current); err != nil {
		return fmt.Errorf("save restored attachments: %w", err)
	}
	return nil
}
