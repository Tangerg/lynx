package runmaintenance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/scope/core/chat"
)

var (
	// ErrModelContextDiverged reports that the Interaction candidate no longer
	// begins with the exact durable conversation snapshot.
	ErrModelContextDiverged = errors.New("runmaintenance: model context diverged from durable conversation")
	// ErrModelContextCannotFit reports that only protected or fixed context is
	// over budget, so no truthful compaction can make the request executable.
	ErrModelContextCannotFit = errors.New("runmaintenance: protected model context cannot fit")
	// ErrModelContextCompactionVetoed reports that a lifecycle hook blocked a
	// compaction required to keep the next model request inside its budget.
	ErrModelContextCompactionVetoed = errors.New("runmaintenance: required model context compaction was vetoed")
)

// CompactModelContext reduces the exact mutable context for one imminent model
// call. Durable root history is reconciled and rewritten transactionally;
// transient child history is reduced only in the returned Interaction state.
func (c *Compactor) CompactModelContext(
	ctx context.Context,
	request agentexec.ModelContextCompaction,
) (agentexec.ModelContextCompactionResult, error) {
	if c == nil {
		return agentexec.ModelContextCompactionResult{}, errors.New("runmaintenance: model-context compactor is nil")
	}
	candidate := request.Candidate()
	history := candidate
	protectedTail := request.ProtectedTail()
	var ephemeral []chat.Message
	if request.Durable() {
		if c.store == nil {
			return agentexec.ModelContextCompactionResult{}, errors.New("runmaintenance: durable compaction store is unavailable")
		}
		stored, err := c.store.Read(ctx, request.SessionID())
		if err != nil {
			return agentexec.ModelContextCompactionResult{}, fmt.Errorf("runmaintenance: read model context: %w", err)
		}
		candidatePrefix, matches, difference, err := semanticMessagePrefix(candidate, stored)
		if err != nil {
			return agentexec.ModelContextCompactionResult{}, err
		}
		if !matches {
			return agentexec.ModelContextCompactionResult{}, fmt.Errorf(
				"%w: candidate_messages=%d durable_messages=%d first_difference=%s",
				ErrModelContextDiverged,
				len(candidate),
				len(stored),
				difference,
			)
		}
		if protectedTail > len(stored) {
			return agentexec.ModelContextCompactionResult{}, fmt.Errorf(
				"%w: protected durable tail %d exceeds stored history %d",
				ErrModelContextDiverged,
				protectedTail,
				len(stored),
			)
		}
		history = stored
		ephemeral = cloneMessages(candidate[candidatePrefix:])
	}

	limits, _, err := modelcatalog.LookupTokenLimits(request.ModelSelection())
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, err
	}
	options := request.Options()
	trigger, err := c.tokenTrigger(limits, options)
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, fmt.Errorf(
			"runmaintenance: resolve model-context token trigger: %w",
			err,
		)
	}
	budget := newModelContextBudget(
		c.maxMessages,
		trigger,
		request.Instructions(),
		ephemeral,
		request.Tools(),
		options,
		request.TokenEstimateAdjustment(),
		newModelContextCounter(request),
	)
	plan, err := c.planCompactionWithProtectedTail(ctx, history, budget, protectedTail)
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, err
	}
	if plan.action == noCompaction {
		if plan.required {
			return agentexec.ModelContextCompactionResult{}, ErrModelContextCannotFit
		}
		return agentexec.NewModelContextCompactionResult(
			candidate,
			false,
			false,
			len(candidate),
			plan.inputTokens,
		)
	}
	if !request.AllowsCompaction(ctx) {
		return agentexec.ModelContextCompactionResult{}, ErrModelContextCompactionVetoed
	}

	replacement, summarized, cutoff, prefixAfter, err := c.materializeModelContextPlan(
		ctx,
		request.SessionID(),
		plan,
	)
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, err
	}
	overBudget, inputTokens, err := budget.exceeded(ctx, replacement)
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, err
	}
	if overBudget {
		return agentexec.ModelContextCompactionResult{}, ErrModelContextCannotFit
	}
	effective := append(cloneMessages(replacement), ephemeral...)
	result, err := agentexec.NewModelContextCompactionResult(
		effective,
		true,
		summarized,
		len(candidate),
		inputTokens,
	)
	if err != nil {
		return agentexec.ModelContextCompactionResult{}, err
	}
	if request.Durable() {
		if err := c.store.RewriteForCompaction(
			ctx,
			request.SessionID(),
			len(history),
			cutoff,
			prefixAfter,
			replacement...,
		); err != nil {
			return agentexec.ModelContextCompactionResult{}, fmt.Errorf(
				"runmaintenance: persist model context compaction: %w",
				err,
			)
		}
	}
	return result, nil
}

type modelContextCounter agentexec.ModelContextCompaction

func newModelContextCounter(request agentexec.ModelContextCompaction) modelContextInputTokenCounter {
	if !request.HasInputTokenCounter() {
		return nil
	}
	return modelContextCounter(request)
}

func (m modelContextCounter) CountInputTokens(
	ctx context.Context,
	messages []chat.Message,
) (int64, error) {
	return agentexec.ModelContextCompaction(m).CountInputTokens(ctx, messages)
}

func (c *Compactor) materializeModelContextPlan(
	ctx context.Context,
	sessionID string,
	plan compactionPlan,
) (
	replacement []chat.Message,
	summarized bool,
	cutoff int,
	prefixAfter int,
	err error,
) {
	switch plan.action {
	case trimCompaction:
		return cloneMessages(plan.trimmed), false, 0, 0, nil
	case summarizeCompaction:
		summary, err := c.summarize(ctx, plan.older)
		if err != nil {
			return nil, false, 0, 0, fmt.Errorf("runmaintenance: summarize model context: %w", err)
		}
		replacement = make([]chat.Message, 0, 2+len(plan.recent))
		replacement = append(replacement, summary)
		if c.liveState != nil {
			if reminder, ok := liveStateReminder(c.liveState(ctx, sessionID)); ok {
				replacement = append(replacement, reminder)
			}
		}
		replacement = append(replacement, cloneMessages(plan.recent)...)
		return replacement, true, plan.cutoff, len(replacement) - len(plan.recent), nil
	default:
		return nil, false, 0, 0, errors.New("runmaintenance: unsupported model-context compaction plan")
	}
}

func cloneMessages(messages []chat.Message) []chat.Message {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

type semanticMessage struct {
	message   chat.Message
	sourceEnd int
}

func semanticMessagePrefix(candidate, durable []chat.Message) (int, bool, string, error) {
	candidateMessages, err := normalizedSemanticMessages(candidate, "Interaction conversation")
	if err != nil {
		return 0, false, "candidate_invalid", err
	}
	durableMessages, err := normalizedSemanticMessages(durable, "durable conversation")
	if err != nil {
		return 0, false, "durable_invalid", err
	}
	if len(candidateMessages) < len(durableMessages) {
		return 0, false, fmt.Sprintf(
			"semantic_message_count=%d/%d",
			len(candidateMessages),
			len(durableMessages),
		), nil
	}
	for index := range durableMessages {
		left := candidateMessages[index].message
		right := durableMessages[index].message
		if left.Role != right.Role {
			return 0, false, fmt.Sprintf(
				"message[%d].role=%s/%s",
				index,
				left.Role,
				right.Role,
			), nil
		}
		if len(left.Parts) != len(right.Parts) {
			return 0, false, fmt.Sprintf(
				"message[%d].part_count=%d/%d",
				index,
				len(left.Parts),
				len(right.Parts),
			), nil
		}
		for partIndex := range left.Parts {
			leftPart := left.Parts[partIndex]
			rightPart := right.Parts[partIndex]
			if !reflect.DeepEqual(leftPart, rightPart) {
				return 0, false, fmt.Sprintf(
					"message[%d].part[%d] kind=%s/%s text_equal=%t signature_equal=%t media_equal=%t tool_call_equal=%t tool_result_equal=%t",
					index,
					partIndex,
					leftPart.Kind,
					rightPart.Kind,
					leftPart.Text == rightPart.Text,
					slices.Equal(leftPart.Signature, rightPart.Signature),
					reflect.DeepEqual(leftPart.Media, rightPart.Media),
					reflect.DeepEqual(leftPart.ToolCall, rightPart.ToolCall),
					reflect.DeepEqual(leftPart.ToolResult, rightPart.ToolResult),
				), nil
			}
		}
	}
	if len(durableMessages) == 0 {
		return 0, true, "none", nil
	}
	return candidateMessages[len(durableMessages)-1].sourceEnd, true, "none", nil
}

func normalizedSemanticMessages(messages []chat.Message, owner string) ([]semanticMessage, error) {
	normalized := make([]semanticMessage, 0, len(messages))
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("runmaintenance: %s message %d: %w", owner, index, err)
		}
		message := messages[index].Clone()
		message.Metadata = nil
		for partIndex := range message.Parts {
			message.Parts[partIndex].Metadata = nil
		}
		if message.Role == chat.RoleTool && len(normalized) > 0 &&
			normalized[len(normalized)-1].message.Role == chat.RoleTool {
			last := &normalized[len(normalized)-1]
			last.message.Parts = append(last.message.Parts, message.Parts...)
			last.sourceEnd = index + 1
			continue
		}
		normalized = append(normalized, semanticMessage{message: message, sourceEnd: index + 1})
	}
	return normalized, nil
}

var _ agentexec.ModelContextCompactor = (*Compactor)(nil)
