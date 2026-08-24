package runflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type pendingConversationCall struct {
	runID string
	call  chat.ToolCall
}

type conversationToolObservation struct {
	value agentexec.ToolObservation
	used  bool
}

func projectConversation(
	record rundomain.Record,
	output agentexec.Output,
	items map[string]transcript.Record,
	journal []conversationdomain.Record,
) ([]conversationdomain.Record, error) {
	pending, err := pendingConversationCalls(journal)
	if err != nil {
		return nil, err
	}
	observations := make(map[string]*conversationToolObservation, len(output.Tools))
	for _, observation := range output.Tools {
		if observation.RunID != record.Run.ID() || observation.CallID == "" {
			return nil, errors.New("runflow: Conversation Tool observation changed source")
		}
		if _, duplicate := observations[observation.CallID]; duplicate {
			return nil, errors.New("runflow: Conversation Tool observation is duplicated")
		}
		observations[observation.CallID] = &conversationToolObservation{value: observation}
	}
	nextOrdinal := nextConversationOrdinal(journal)
	projected := make([]conversationdomain.Record, 0, len(output.Models)*2+1)
	appendMessage := func(message chat.Message) error {
		body, err := json.Marshal(message)
		if err != nil {
			return err
		}
		projected = append(projected, conversationdomain.Record{
			SessionID: record.Run.SessionID(), RunID: record.Run.ID(),
			Ordinal: nextOrdinal, Body: body,
		})
		nextOrdinal++
		return nil
	}
	settled := make([]chat.ToolResult, 0, len(pending))
	unresolvedPending := false
	for _, pendingCall := range pending {
		if pendingCall.runID != record.Run.ID() {
			return nil, errors.New("runflow: a predecessor Run left an open Conversation ToolCall")
		}
		result, found, err := resolveConversationToolResult(
			record.Run.SessionID(), pendingCall.runID, pendingCall.call, items, observations,
		)
		if err != nil {
			return nil, err
		}
		if found {
			settled = append(settled, result)
		} else {
			unresolvedPending = true
		}
	}
	if len(settled) > 0 {
		if err := appendMessage(chat.NewToolMessage(settled...)); err != nil {
			return nil, err
		}
	}
	models := append([]agentexec.ModelObservation(nil), output.Models...)
	sort.SliceStable(models, func(left, right int) bool {
		return models[left].Sequence < models[right].Sequence
	})
	if unresolvedPending && len(models) > 0 {
		return nil, errors.New("runflow: model advanced past an open Conversation ToolCall")
	}
	for _, model := range models {
		if model.RunID != record.Run.ID() || model.Response == nil {
			return nil, errors.New("runflow: Conversation model observation changed source")
		}
		choice := model.Response.First()
		if choice == nil || choice.Message == nil {
			continue
		}
		message := choice.Message.Clone()
		if err := appendMessage(message); err != nil {
			return nil, err
		}
		results := make([]chat.ToolResult, 0)
		for _, part := range message.Parts {
			if part.Kind != chat.PartToolCall || part.ToolCall == nil {
				continue
			}
			result, found, err := resolveConversationToolResult(
				record.Run.SessionID(), record.Run.ID(), *part.ToolCall, items, observations,
			)
			if err != nil {
				return nil, err
			}
			if found {
				results = append(results, result)
			}
		}
		if len(results) > 0 {
			if err := appendMessage(chat.NewToolMessage(results...)); err != nil {
				return nil, err
			}
		}
	}
	for _, observation := range observations {
		if !observation.used && !observation.value.Waiting {
			return nil, errors.New("runflow: settled Tool has no Conversation ToolCall")
		}
	}
	return projected, nil
}

func pendingConversationCalls(
	journal []conversationdomain.Record,
) ([]pendingConversationCall, error) {
	ordered := make([]pendingConversationCall, 0)
	open := make(map[string]bool)
	for _, record := range journal {
		var message chat.Message
		if err := json.Unmarshal(record.Body, &message); err != nil {
			return nil, fmt.Errorf("runflow: decode Conversation message: %w", err)
		}
		for _, part := range message.Parts {
			switch part.Kind {
			case chat.PartToolCall:
				key := conversationCallKey(record.RunID, part.ToolCall.ID)
				if open[key] {
					return nil, errors.New("runflow: Conversation repeats an open ToolCall")
				}
				open[key] = true
				ordered = append(ordered, pendingConversationCall{runID: record.RunID, call: *part.ToolCall})
			case chat.PartToolResult:
				key := conversationCallKey(record.RunID, part.ToolResult.ID)
				if !open[key] {
					return nil, errors.New("runflow: Conversation ToolResult has no open ToolCall")
				}
				delete(open, key)
			}
		}
	}
	pending := ordered[:0]
	for _, call := range ordered {
		if open[conversationCallKey(call.runID, call.call.ID)] {
			pending = append(pending, call)
		}
	}
	return pending, nil
}

func resolveConversationToolResult(
	sessionID, runID string,
	call chat.ToolCall,
	items map[string]transcript.Record,
	observations map[string]*conversationToolObservation,
) (chat.ToolResult, bool, error) {
	if observation := observations[call.ID]; observation != nil {
		if observation.used {
			return chat.ToolResult{}, false, errors.New("runflow: Conversation Tool result was consumed twice")
		}
		if observation.value.Waiting {
			return chat.ToolResult{}, false, nil
		}
		observation.used = true
		result := observation.value.Result
		isError := observation.value.IsError
		if observation.value.Failure != "" {
			result = "error: " + observation.value.Failure
			isError = true
		} else if projected := toolresult.Project(observation.value.ItemID, result); projected.Offloaded {
			result = projected.Preview
		}
		return chat.ToolResult{ID: call.ID, Name: call.Name, Result: result, IsError: isError}, true, nil
	}
	stored, found := items[agentexec.ToolItemID(runID, call.ID)]
	if !found {
		if call.Name != agentexec.DelegateToolName {
			return chat.ToolResult{}, false, nil
		}
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: "error: delegated worker request was rejected", IsError: true,
		}, true, nil
	}
	var item protocol.Item
	if err := json.Unmarshal(stored.Body, &item); err != nil {
		return chat.ToolResult{}, false, err
	}
	if item.RunID != runID {
		return chat.ToolResult{}, false, errors.New("runflow: Conversation Item changed source")
	}
	switch item.Status {
	case protocol.ItemStatusRunning:
		return chat.ToolResult{}, false, nil
	case protocol.ItemStatusCompleted:
		if call.Name != agentexec.DelegateToolName {
			if item.Tool == nil || item.Tool.Name != call.Name {
				return chat.ToolResult{}, false, errors.New("runflow: completed Conversation Item changed Tool identity")
			}
			result, err := conversationResultText(item.Tool.Result)
			if err != nil {
				return chat.ToolResult{}, false, err
			}
			return chat.ToolResult{ID: call.ID, Name: call.Name, Result: result}, true, nil
		}
		if item.Type != protocol.ItemTypeToolCall || item.Tool == nil ||
			item.Tool.Name != agentexec.DelegateToolName {
			return chat.ToolResult{}, false, errors.New("runflow: Delegate Conversation Item changed identity")
		}
		var material struct {
			Reply string `json:"reply"`
		}
		body, err := json.Marshal(item.Tool.Result)
		if err != nil {
			return chat.ToolResult{}, false, err
		}
		if err := json.Unmarshal(body, &material); err != nil || material.Reply == "" {
			return chat.ToolResult{}, false, errors.New("runflow: completed Delegate Item has no reply")
		}
		body, err = json.Marshal(struct {
			Reply string `json:"reply"`
		}{Reply: material.Reply})
		if err != nil {
			return chat.ToolResult{}, false, err
		}
		return chat.ToolResult{ID: call.ID, Name: call.Name, Result: string(body)}, true, nil
	case protocol.ItemStatusIncomplete:
		detail := "did not complete"
		if item.Error != nil && item.Error.Detail != "" {
			detail = item.Error.Detail
		}
		prefix := "error: "
		if call.Name == agentexec.DelegateToolName {
			prefix = "error: delegated worker "
		}
		return chat.ToolResult{
			ID: call.ID, Name: call.Name,
			Result: prefix + detail, IsError: true,
		}, true, nil
	default:
		return chat.ToolResult{}, false, errors.New("runflow: Conversation Item has invalid status")
	}
}

func conversationCallKey(runID, callID string) string {
	return runID + "\x00" + callID
}

func conversationResultText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
