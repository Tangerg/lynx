package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	corechat "github.com/Tangerg/scope/core/chat"
	coremetadata "github.com/Tangerg/scope/core/metadata"
)

const rootOpeningMessageCount = 1

type interactionModelContextReducer struct {
	compactor    ModelContextCompactor
	session      *interactionSession
	start        runs.RootExecutionStart
	instructions []corechat.Message
}

func newInteractionModelContextReducer(
	compactor ModelContextCompactor,
	session *interactionSession,
	start runs.RootExecutionStart,
	instructions []corechat.Message,
) *interactionModelContextReducer {
	return &interactionModelContextReducer{
		compactor:    compactor,
		session:      session,
		start:        start,
		instructions: cloneChatMessages(instructions),
	}
}

func (i *interactionModelContextReducer) ReduceModelContext(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	request *corechat.Request,
) ([]corechat.Message, error) {
	if i == nil || i.compactor == nil {
		return nil, errors.New("agentexec: model-context compactor is unavailable")
	}
	if i.session == nil || request == nil || !invocation.Valid() {
		return nil, errors.New("agentexec: model-context reduction requires an attributed Interaction request")
	}
	prefixMatches, err := sameInteractionMessages(
		request.Messages[:min(len(request.Messages), len(i.instructions))],
		i.instructions,
	)
	if err != nil {
		return nil, err
	}
	if len(request.Messages) <= len(i.instructions) || !prefixMatches {
		return nil, fmt.Errorf(
			"agentexec: model context no longer starts with its frozen instructions (messages=%d instructions=%d prefix_matches=%t)",
			len(request.Messages),
			len(i.instructions),
			prefixMatches,
		)
	}
	candidate := cloneChatMessages(request.Messages[len(i.instructions):])
	preCompact := func(ctx context.Context) bool {
		return i.session.lifecycleHooks == nil || i.session.lifecycleHooks.BeforeCompaction(
			ctx,
			i.start.SessionID,
			i.start.CWD,
		)
	}

	var (
		compaction ModelContextCompaction
		buildErr   error
	)
	if invocation.Relation().IsRoot() {
		protectedTail := 0
		if invocation.ModelCallSequence() == 1 {
			protectedTail = rootOpeningMessageCount
		}
		compaction, buildErr = NewDurableModelContextCompaction(
			i.start.SessionID,
			i.start.ModelSelection,
			i.instructions,
			candidate,
			request.Tools,
			request.Options,
			protectedTail,
			preCompact,
		)
	} else {
		protectedTail := 0
		if invocation.ModelCallSequence() == 1 {
			protectedTail = len(candidate)
		} else if signalCount := len(invocation.AppliedSteerSignalIDs()); signalCount > 0 {
			protectedTail = trailingUserMessageCount(candidate)
			if protectedTail < signalCount {
				return nil, fmt.Errorf(
					"agentexec: delegated model context attributes %d steer Signals but has only %d trailing User messages",
					signalCount,
					protectedTail,
				)
			}
		}
		compaction, buildErr = NewTransientModelContextCompaction(
			i.start.SessionID,
			i.start.ModelSelection,
			i.instructions,
			candidate,
			request.Tools,
			request.Options,
			protectedTail,
			preCompact,
		)
	}
	if buildErr != nil {
		return nil, buildErr
	}
	result, err := i.compactor.CompactModelContext(ctx, compaction)
	if err != nil {
		return nil, err
	}
	effective := append(
		cloneChatMessages(i.instructions),
		result.Messages()...,
	)
	validation := request.Clone()
	validation.Messages = effective
	if err := validation.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: compacted model context: %w", err)
	}
	if result.Summarized() {
		before, after := result.MessageCounts()
		i.session.lifetime.send(runs.ExecutorEvent{
			Member: i.session.executorMember(invocation.Relation()),
			Payload: runs.CompactionBoundary{
				MessagesBefore: before,
				MessagesAfter:  after,
			},
		})
	}
	return effective, nil
}

func trailingUserMessageCount(messages []corechat.Message) int {
	count := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != corechat.RoleUser {
			break
		}
		count++
	}
	return count
}

func sameInteractionMessages(left, right []corechat.Message) (bool, error) {
	if len(left) != len(right) {
		return false, nil
	}
	for index := range left {
		if err := left[index].Validate(); err != nil {
			return false, fmt.Errorf("agentexec: effective instruction %d: %w", index, err)
		}
		if err := right[index].Validate(); err != nil {
			return false, fmt.Errorf("agentexec: frozen instruction %d: %w", index, err)
		}
		if left[index].Role != right[index].Role ||
			!sameInteractionMetadata(left[index].Metadata, right[index].Metadata) ||
			len(left[index].Parts) != len(right[index].Parts) {
			return false, nil
		}
		for partIndex := range left[index].Parts {
			leftPart := left[index].Parts[partIndex]
			rightPart := right[index].Parts[partIndex]
			if leftPart.Kind != rightPart.Kind || leftPart.Text != rightPart.Text ||
				!sameInteractionMetadata(leftPart.Metadata, rightPart.Metadata) {
				return false, nil
			}
		}
	}
	return true, nil
}

func sameInteractionMetadata(left, right coremetadata.Map) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, found := right[key]
		if !found {
			return false
		}
		var leftDecoded, rightDecoded any
		if json.Unmarshal(leftValue, &leftDecoded) != nil ||
			json.Unmarshal(rightValue, &rightDecoded) != nil ||
			!reflect.DeepEqual(leftDecoded, rightDecoded) {
			return false
		}
	}
	return true
}

var _ interaction.ModelContextReducer = (*interactionModelContextReducer)(nil)
