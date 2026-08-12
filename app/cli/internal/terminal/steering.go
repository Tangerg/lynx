package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	request := agent.SteerRun{RunID: runID, SegmentID: segmentID, Message: message}
	if err := request.Validate(); err != nil {
		return err
	}

	// The command text has already been removed. Take ownership of its staged
	// attachments now so later typing starts a fresh draft; a refused steer merges
	// them back instead of overwriting text entered while the call was in flight.
	a.resetComposer()
	started := runOperation(a, steerRunOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, a.runtime.SteerRun(ctx, request)
		},
		func(_ struct{}, err error) {
			if err != nil {
				a.restoreSteerAttachments(message.Attachments)
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
		a.restoreSteerAttachments(message.Attachments)
		return errors.New("another steer operation is already running")
	}
	return nil
}

func (a *app) restoreSteerAttachments(rejected []agent.Attachment) {
	if len(rejected) == 0 {
		return
	}
	current, _, err := a.currentDraft()
	if err != nil {
		a.message(fmt.Sprintf("steer attachments could not be restored: %v", err))
		return
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
	a.restoreComposer(current)
	a.persistDraft()
}
