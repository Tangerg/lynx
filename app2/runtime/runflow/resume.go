package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

var ErrInterruptSetNotFound = errors.New("runflow: interrupt set not found")

// ResumeWrite is one durable continuation boundary. The adapter must compare
// ExpectedInterrupts and consume the set in the same transaction that opens the
// new segment, updates answered Items, and appends opening events.
type ResumeWrite struct {
	Run                rundomain.Record
	ExpectedInterrupts protocol.PendingInterruptSet
	UpdatedItems       []transcript.Record
	OpeningItem        *transcript.Record
	OpeningMessage     *conversationdomain.Record
	Events             []rundomain.EventRecord
}

type ResumeCommand struct {
	Request      protocol.ResumeRunRequest
	BeforeLaunch func(context.Context, string) error
}

func (service *Service) Resume(ctx context.Context, request protocol.ResumeRunRequest) (
	*protocol.ResumeRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	return service.ResumeWith(ctx, ResumeCommand{Request: request})
}

func (service *Service) ResumeWith(ctx context.Context, command ResumeCommand) (
	*protocol.ResumeRunResponse,
	iter.Seq[protocol.RunEvent],
	error,
) {
	request := command.Request
	lock := service.runLock(request.RunID)
	lock.Lock()
	defer lock.Unlock()

	record, err := service.store.GetRun(ctx, request.RunID)
	if err != nil {
		return nil, nil, projectRunLookup(err)
	}
	if record.Run.ParentRunID() != "" {
		return nil, nil, protocol.ErrRunNotRoot
	}
	if record.Run.Status() != rundomain.Waiting {
		return nil, nil, protocol.ErrInterruptNotOpen
	}
	pending, err := service.store.GetInterruptSet(ctx, request.RunID)
	if errors.Is(err, ErrInterruptSetNotFound) {
		return nil, nil, protocol.ErrInterruptNotOpen
	}
	if err != nil {
		return nil, nil, err
	}
	if pending.RootRunID != request.RunID || pending.SessionID != record.Run.SessionID() {
		return nil, nil, errors.New("runflow: interrupt ownership differs from waiting run")
	}
	answers, err := validateResumeResponses(pending, request.Responses)
	if err != nil {
		return nil, nil, err
	}

	items, err := service.store.ListItems(ctx, record.Run.SessionID(), "")
	if err != nil {
		return nil, nil, err
	}
	updated, err := resolveInterruptItems(items, pending, answers, service.now().UTC())
	if err != nil {
		return nil, nil, err
	}
	checkpoint, err := service.store.GetExecutorCheckpoint(ctx, request.RunID)
	if err != nil {
		return nil, nil, protocol.ErrInterruptNotOpen
	}
	frameworkResponse, err := frameworkResumeResponse(pending, answers)
	if err != nil {
		return nil, nil, err
	}

	segmentID, err := service.ids.New("seg_")
	if err != nil {
		return nil, nil, err
	}
	now := service.now().UTC()
	if err := record.Run.Resume(segmentID, now); err != nil {
		return nil, nil, err
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return nil, nil, err
	}
	var opening *transcript.Record
	var openingMessage *conversationdomain.Record
	var userItemID *string
	if len(request.Input) > 0 {
		id, idErr := service.ids.New("itm_")
		if idErr != nil {
			return nil, nil, idErr
		}
		ordinal := nextOrdinal(items, request.RunID)
		item := protocol.Item{ID: id, RunID: request.RunID, Status: protocol.ItemStatusCompleted, CreatedAt: now, Type: protocol.ItemTypeUserMessage, Content: slices.Clone(request.Input)}
		stored, storeErr := itemRecord(record.Run.SessionID(), item, ordinal)
		if storeErr != nil {
			return nil, nil, storeErr
		}
		opening = &stored
		userItemID = &id
		message, messageErr := agentexec.UserMessage(request.Input)
		if messageErr != nil { return nil, nil, messageErr }
		body, messageErr := json.Marshal(message)
		if messageErr != nil { return nil, nil, messageErr }
		messages, messageErr := service.store.ListConversationMessages(ctx, record.Run.SessionID())
		if messageErr != nil { return nil, nil, messageErr }
		value := conversationdomain.Record{SessionID: record.Run.SessionID(), RunID: request.RunID, Ordinal: nextConversationOrdinal(messages), Body: body}
		openingMessage = &value
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return nil, nil, err
	}
	presented, err := presentRecord(record)
	if err != nil {
		return nil, nil, err
	}
	events, persisted, err := service.resumeEvents(*presented, opening, &facts, now)
	if err != nil {
		return nil, nil, err
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return nil, nil, err
	}
	if err := service.store.CommitResume(ctx, ResumeWrite{Run: record, ExpectedInterrupts: pending, UpdatedItems: updated, OpeningItem: opening, OpeningMessage: openingMessage, Events: persisted}); err != nil {
		if errors.Is(err, ErrInterruptSetNotFound) {
			return nil, nil, protocol.ErrInterruptNotOpen
		}
		return nil, nil, err
	}
	if command.BeforeLaunch != nil {
		if err := command.BeforeLaunch(ctx, request.RunID); err != nil {
			service.settleUnlaunched(request.RunID)
			return nil, nil, fmt.Errorf("runflow: prepare resumed Run ownership: %w", err)
		}
	}
	storedSession, err := service.store.GetSession(ctx, session.ID(record.Run.SessionID()))
	if err != nil {
		return nil, nil, err
	}
	stream := service.hub.SubscribeRun(ctx, request.RunID, segmentID, events)
	if !service.launchResumeExecution(record, segmentID, storedSession.Workspace().Path(), checkpoint, frameworkResponse, request.Input) {
		service.settleUnlaunched(request.RunID)
	}
	return &protocol.ResumeRunResponse{RunID: request.RunID, SegmentID: segmentID, UserItemID: userItemID}, stream, nil
}

func (service *Service) launchResumeExecution(record rundomain.Record, segmentID, workspace string, checkpoint, response []byte, additionalInput []protocol.ContentBlock) bool {
	service.mu.Lock()
	if service.closing {
		service.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(service.lifetime)
	steers := make(chan agentexec.Steer, 32)
	service.active[record.Run.ID()] = activeExecution{cancel: cancel, steers: steers}
	service.tasks.Add(1)
	service.mu.Unlock()
	go func() {
		defer service.tasks.Done()
		defer func() {
			service.mu.Lock()
			delete(service.active, record.Run.ID())
			service.mu.Unlock()
			cancel()
		}()
		output, executeErr := service.executor.Resume(ctx, agentexec.ResumeInput{
			Provider: record.Run.Provider(), Model: record.Run.Model(), Workspace: workspace,
			SessionID: record.Run.SessionID(), RunID: record.Run.ID(), IsRootRun: record.Run.ParentRunID() == "", MaxSteps: runMaxSteps(record.Body),
			Checkpoint: checkpoint, Response: response, Steers: steers,
			AdditionalInput: slices.Clone(additionalInput),
		})
		service.finishExecution(record.Run.ID(), segmentID, workspace, output, executeErr)
	}()
	return true
}

func validateResumeResponses(pending protocol.PendingInterruptSet, responses []protocol.InterruptResponse) (map[string]protocol.InterruptResponseValue, error) {
	if len(responses) != len(pending.Interrupts) {
		return nil, fmt.Errorf("%w: responses must exactly cover the open interrupt set", protocol.ErrInvalidParams)
	}
	byID := make(map[string]protocol.InterruptResponseValue, len(responses))
	for _, response := range responses {
		if response.ItemID == "" {
			return nil, fmt.Errorf("%w: response itemId is required", protocol.ErrInvalidParams)
		}
		if _, duplicate := byID[response.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate response for %s", protocol.ErrInvalidParams, response.ItemID)
		}
		byID[response.ItemID] = response.Response
	}
	for _, interrupt := range pending.Interrupts {
		response, ok := byID[interrupt.ItemID]
		if !ok {
			return nil, fmt.Errorf("%w: no response for %s", protocol.ErrInvalidParams, interrupt.ItemID)
		}
		switch interrupt.Type {
		case protocol.InterruptApproval:
			if response.Type != protocol.InterruptResponseApproval || (response.Decision != protocol.ApprovalApprove && response.Decision != protocol.ApprovalDeny) {
				return nil, fmt.Errorf("%w: %s requires an approval response", protocol.ErrInvalidParams, interrupt.ItemID)
			}
			if response.Remember != nil && (interrupt.Payload == nil || !interrupt.Payload.Rememberable) {
				return nil, fmt.Errorf("%w: %s cannot be remembered", protocol.ErrInvalidParams, interrupt.ItemID)
			}
		case protocol.InterruptQuestion:
			if response.Type != protocol.InterruptResponseAnswer || interrupt.Payload == nil || interrupt.Payload.Question == nil {
				return nil, fmt.Errorf("%w: %s requires an answer response", protocol.ErrInvalidParams, interrupt.ItemID)
			}
			if err := validateQuestionAnswers(*interrupt.Payload.Question, response.Answers); err != nil {
				return nil, fmt.Errorf("%w: %s: %v", protocol.ErrInvalidParams, interrupt.ItemID, err)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported interrupt type %q", protocol.ErrInvalidParams, interrupt.Type)
		}
	}
	return byID, nil
}

func validateQuestionAnswers(question protocol.Question, answers [][]string) error {
	if len(answers) != len(question.Fields) {
		return errors.New("answers must match the question field count")
	}
	for index, field := range question.Fields {
		values := answers[index]
		if len(values) == 0 {
			return fmt.Errorf("field %d has no answer", index)
		}
		if field.Type == protocol.QuestionFieldText && (len(values) != 1 || strings.TrimSpace(values[0]) == "") {
			return fmt.Errorf("field %d requires one non-empty text answer", index)
		}
		if field.Type == protocol.QuestionFieldChoice {
			if !field.Multiple && len(values) != 1 {
				return fmt.Errorf("field %d allows one choice", index)
			}
			for _, value := range values {
				known := slices.ContainsFunc(field.Options, func(option protocol.QuestionOption) bool { return option.Label == value })
				if !known && (!field.AllowCustom || strings.TrimSpace(value) == "") {
					return fmt.Errorf("field %d contains an unknown choice", index)
				}
			}
		}
	}
	return nil
}

func resolveInterruptItems(records []transcript.Record, pending protocol.PendingInterruptSet, answers map[string]protocol.InterruptResponseValue, now time.Time) ([]transcript.Record, error) {
	byID := make(map[string]transcript.Record, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	updated := make([]transcript.Record, 0, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		record, ok := byID[interrupt.ItemID]
		if !ok {
			return nil, fmt.Errorf("runflow: interrupt item %s is missing", interrupt.ItemID)
		}
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, err
		}
		response := answers[interrupt.ItemID]
		switch interrupt.Type {
		case protocol.InterruptQuestion:
			if item.Type != protocol.ItemTypeQuestion || item.Question == nil {
				return nil, fmt.Errorf("runflow: interrupt item %s is not a question", item.ID)
			}
			item.Question.Answers = cloneAnswers(response.Answers)
			item.Status = protocol.ItemStatusCompleted
		case protocol.InterruptApproval:
			if item.Type != protocol.ItemTypeToolCall || item.Tool == nil {
				return nil, fmt.Errorf("runflow: interrupt item %s is not a tool call", item.ID)
			}
			item.ApprovalDecision = response.Decision
			if response.EditedArgs != nil {
				item.Tool.Arguments = cloneMap(response.EditedArgs)
			}
			if response.Decision == protocol.ApprovalDeny {
				item.FinishedAt = now
				item.Status = protocol.ItemStatusIncomplete
				item.Error = &protocol.ProblemData{Type: protocol.ProblemDeniedByUser, Detail: strings.TrimSpace(response.Reason)}
			} else {
				// Approval settles only the gate. The ToolCall remains running
				// until the restored Interaction produces its real ToolResult.
				item.Status = protocol.ItemStatusRunning
				item.FinishedAt = time.Time{}
				item.Error = nil
			}
		}
		body, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		record.Body = body
		updated = append(updated, record)
	}
	return updated, nil
}

func (service *Service) resumeEvents(run protocol.RunRef, opening *transcript.Record, facts *runFacts, now time.Time) ([]protocol.RunEvent, []rundomain.EventRecord, error) {
	started, err := service.event(run.ID, run.ActiveSegmentID, facts, protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}, now)
	if err != nil {
		return nil, nil, err
	}
	events := []protocol.RunEvent{started}
	if opening != nil {
		var item protocol.Item
		if err := json.Unmarshal(opening.Body, &item); err != nil {
			return nil, nil, err
		}
		completed, err := service.event(run.ID, run.ActiveSegmentID, facts, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}, now)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, completed)
	}
	persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1)
	return events, persisted, err
}

func frameworkResumeResponse(pending protocol.PendingInterruptSet, answers map[string]protocol.InterruptResponseValue) ([]byte, error) {
	if len(pending.Interrupts) != 1 {
		return nil, fmt.Errorf("runflow: framework continuation requires one active tool input, got %d", len(pending.Interrupts))
	}
	interrupt := pending.Interrupts[0]
	response := answers[interrupt.ItemID]
	switch interrupt.Type {
	case protocol.InterruptApproval:
		return json.Marshal(struct {
			Decision protocol.ApprovalDecision `json:"decision"`
			EditedArgs map[string]any `json:"editedArgs,omitempty"`
			Reason string `json:"reason,omitempty"`
		}{Decision: response.Decision, EditedArgs: response.EditedArgs, Reason: response.Reason})
	case protocol.InterruptQuestion:
		return json.Marshal(struct { Answers [][]string `json:"answers"` }{Answers: response.Answers})
	default:
		return nil, fmt.Errorf("runflow: unsupported framework interrupt %q", interrupt.Type)
	}
}

func nextOrdinal(records []transcript.Record, runID string) int {
	next := 0
	for _, record := range records {
		if record.RunID == runID && record.Ordinal >= next {
			next = record.Ordinal + 1
		}
	}
	return next
}

func cloneAnswers(values [][]string) [][]string {
	result := make([][]string, len(values))
	for index := range values {
		result[index] = slices.Clone(values[index])
	}
	return result
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
