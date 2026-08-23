package runflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type executionProjection struct {
	items  []transcript.Record
	messages []conversationdomain.Record
	results []toolresult.Record
	events []protocol.RunEvent
}

func (service *Service) projectExecution(ctx context.Context, record rundomain.Record, segmentID string, output agentexec.Output, facts *runFacts, terminal bool) (executionProjection, error) {
	existing, err := service.store.ListItems(ctx, "", record.Run.ID())
	if err != nil {
		return executionProjection{}, err
	}
	byID := make(map[string]transcript.Record, len(existing))
	for _, value := range existing {
		byID[value.ID] = value
	}
	ordinal := nextOrdinal(existing, record.Run.ID())
	conversation, err := service.store.ListConversationMessages(ctx, record.Run.SessionID())
	if err != nil { return executionProjection{}, err }
	messageOrdinal := nextConversationOrdinal(conversation)
	sequences := observationSequences(output.Models, output.Tools)
	finalSequence := 0
	if terminal {
		for _, model := range output.Models {
			if !responseHasToolCalls(model.Response) && model.Sequence > finalSequence {
				finalSequence = model.Sequence
			}
		}
	}
	projection := executionProjection{}
	offloadedPreviews := make(map[string]string)
	for _, sequence := range sequences {
		for _, model := range output.Models {
			if model.Sequence != sequence {
				continue
			}
			choice := model.Response.First()
			if choice != nil && choice.Message != nil {
				body, err := json.Marshal(choice.Message)
				if err != nil { return executionProjection{}, err }
				projection.messages = append(projection.messages, conversationdomain.Record{SessionID: record.Run.SessionID(), RunID: record.Run.ID(), Ordinal: messageOrdinal, Body: body})
				messageOrdinal++
			}
			items, err := modelItems(record.Run.ID(), model, model.Sequence == finalSequence)
			if err != nil {
				return executionProjection{}, err
			}
			for _, item := range items {
				if _, duplicate := byID[item.ID]; duplicate {
					continue
				}
				stored, err := itemRecord(record.Run.SessionID(), item, ordinal)
				if err != nil {
					return executionProjection{}, err
				}
				ordinal++
				byID[item.ID] = stored
				projection.items = append(projection.items, stored)
				event, err := service.event(record.Run.ID(), segmentID, facts, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}, item.CreatedAt)
				if err != nil {
					return executionProjection{}, err
				}
				projection.events = append(projection.events, event)
			}
		}
		for _, observation := range output.Tools {
			if observation.ModelCallSequence != sequence || observation.Waiting {
				continue
			}
			stored, existed := byID[observation.ItemID]
			item, offload, skip, err := toolItem(stored, existed, record.Run.SessionID(), record.Run.ID(), observation)
			if err != nil {
				return executionProjection{}, err
			}
			if skip {
				continue
			}
			if !existed {
				stored, err = itemRecord(record.Run.SessionID(), item, ordinal)
				if err != nil {
					return executionProjection{}, err
				}
				ordinal++
				running := item
				running.Status = protocol.ItemStatusRunning
				running.FinishedAt = time.Time{}
				running.DurationMillis = nil
				running.Tool.Result = nil
				running.Error = nil
				started, err := service.event(record.Run.ID(), segmentID, facts, protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &running}, observation.StartedAt)
				if err != nil {
					return executionProjection{}, err
				}
				projection.events = append(projection.events, started)
			} else {
				stored.Body, err = json.Marshal(item)
				if err != nil {
					return executionProjection{}, err
				}
			}
			byID[item.ID] = stored
			projection.items = append(projection.items, stored)
			if offload != nil { projection.results = append(projection.results, *offload); offloadedPreviews[item.ID] = offload.Preview }
			completed, err := service.event(record.Run.ID(), segmentID, facts, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}, observation.FinishedAt)
			if err != nil {
				return executionProjection{}, err
			}
			projection.events = append(projection.events, completed)
			if observation.CommittedPlan != nil {
				plan := *observation.CommittedPlan
				if plan.SessionID != record.Run.SessionID() {
					return executionProjection{}, errors.New("runflow: committed Plan belongs to another session")
				}
				updated, err := service.event(record.Run.ID(), segmentID, facts, protocol.StreamEvent{Type: protocol.StreamPlanUpdated, Plan: &plan}, observation.FinishedAt)
				if err != nil {
					return executionProjection{}, err
				}
				projection.events = append(projection.events, updated)
			}
		}
		results := make([]chat.ToolResult, 0)
		for _, observation := range output.Tools {
			if observation.ModelCallSequence != sequence || observation.Waiting || observation.Failure != "" { continue }
			result := observation.Result
			if preview, offloaded := offloadedPreviews[observation.ItemID]; offloaded { result = preview }
			results = append(results, chat.ToolResult{ID: observation.CallID, Name: observation.Name, Result: result, IsError: observation.IsError})
		}
		if len(results) > 0 {
			message := chat.NewToolMessage(results...)
			body, err := json.Marshal(message)
			if err != nil { return executionProjection{}, err }
			projection.messages = append(projection.messages, conversationdomain.Record{SessionID: record.Run.SessionID(), RunID: record.Run.ID(), Ordinal: messageOrdinal, Body: body})
			messageOrdinal++
		}
	}
	return projection, nil
}

func nextConversationOrdinal(records []conversationdomain.Record) int {
	next := 0
	for _, record := range records { if record.Ordinal >= next { next = record.Ordinal + 1 } }
	return next
}

func observationSequences(models []agentexec.ModelObservation, tools []agentexec.ToolObservation) []int {
	set := make(map[int]struct{}, len(models)+len(tools))
	for _, value := range models { set[value.Sequence] = struct{}{} }
	for _, value := range tools { set[value.ModelCallSequence] = struct{}{} }
	values := make([]int, 0, len(set))
	for value := range set { values = append(values, value) }
	sort.Ints(values)
	return values
}

func modelItems(runID string, observation agentexec.ModelObservation, final bool) ([]protocol.Item, error) {
	choice := observation.Response.First()
	if choice == nil || choice.Message == nil {
		return nil, nil
	}
	content := make([]protocol.ContentBlock, 0)
	items := make([]protocol.Item, 0)
	effectID := observation.EffectID
	if effectID == "" {
		effectID = fmt.Sprintf("sequence:%d", observation.Sequence)
	}
	reasoningIndex := 0
	for _, part := range choice.Message.Parts {
		switch part.Kind {
		case chat.PartText:
			if part.Text != "" { content = append(content, protocol.ContentBlock{Type: protocol.ContentBlockText, Text: part.Text}) }
		case chat.PartMedia:
			if part.Media == nil || part.Media.Source.Kind != "bytes" || !strings.HasPrefix(part.Media.MIME, "image/") {
				continue
			}
			content = append(content, protocol.ContentBlock{Type: protocol.ContentBlockImage, Mime: part.Media.MIME, Data: base64.StdEncoding.EncodeToString(part.Media.Source.Bytes)})
		case chat.PartReasoning:
			items = append(items, protocol.Item{
				ID: modelReasoningItemID(runID, effectID, reasoningIndex),
				RunID: runID, Status: protocol.ItemStatusCompleted, CreatedAt: observation.OccurredAt,
				Type: protocol.ItemTypeReasoning, Text: part.Text, Redacted: part.Text == "" && len(part.Signature) > 0,
			})
			reasoningIndex++
		}
	}
	if len(content) > 0 {
		phase := protocol.MessagePhaseCommentary
		if final { phase = protocol.MessagePhaseFinalAnswer }
		items = append(items, protocol.Item{
			ID: modelMessageItemID(runID, effectID),
			RunID: runID, Status: protocol.ItemStatusCompleted, CreatedAt: observation.OccurredAt,
			Type: protocol.ItemTypeAgentMessage, Phase: phase, Content: content,
		})
	}
	return items, nil
}

func modelMessageItemID(runID, effectID string) string {
	return stableItemID(runID, "model:"+effectID+":message")
}

func modelReasoningItemID(runID, effectID string, index int) string {
	return stableItemID(runID, fmt.Sprintf("model:%s:reasoning:%d", effectID, index))
}

func responseHasToolCalls(response *chat.Response) bool {
	choice := response.First()
	if choice == nil || choice.Message == nil { return false }
	return slices.ContainsFunc(choice.Message.Parts, func(part chat.Part) bool { return part.Kind == chat.PartToolCall })
}

func toolItem(stored transcript.Record, existed bool, sessionID, runID string, observation agentexec.ToolObservation) (protocol.Item, *toolresult.Record, bool, error) {
	item := protocol.Item{}
	if existed {
		if err := json.Unmarshal(stored.Body, &item); err != nil {
			return protocol.Item{}, nil, false, err
		}
		if item.Type == protocol.ItemTypeQuestion || item.Status != protocol.ItemStatusRunning {
			return protocol.Item{}, nil, true, nil
		}
	} else {
		item = protocol.Item{
			ID: observation.ItemID, RunID: runID, Status: protocol.ItemStatusRunning,
			StartedAt: observation.StartedAt, Type: protocol.ItemTypeToolCall,
			Tool: &protocol.ToolInvocation{Name: observation.Name, Arguments: cloneMap(observation.Arguments)},
			SafetyClass: observation.SafetyClass,
		}
	}
	if item.Tool == nil {
		return protocol.Item{}, nil, false, errors.New("runflow: observed ToolCall item has no invocation")
	}
	item.Tool.Name = observation.Name
	item.Tool.Arguments = cloneMap(observation.Arguments)
	item.FinishedAt = observation.FinishedAt
	duration := observation.FinishedAt.Sub(observation.StartedAt).Milliseconds()
	if duration < 0 { return protocol.Item{}, nil, false, errors.New("runflow: tool finish precedes start") }
	item.DurationMillis = &duration
	if observation.IsError || observation.Failure != "" {
		item.Status = protocol.ItemStatusIncomplete
		detail := observation.Failure
		if detail == "" { detail = observation.Result }
		if len(detail) > 4096 { detail = detail[:4096] }
		problemType := protocol.ProblemToolFailed
		if strings.Contains(strings.ToLower(detail), "canceled") { problemType = protocol.ProblemToolCanceled }
		item.Error = &protocol.ProblemData{Type: problemType, Detail: detail}
		item.Tool.Result = nil
	} else {
		item.Status = protocol.ItemStatusCompleted
		item.Error = nil
		result, offload := projectToolResult(sessionID, item.ID, observation.Name, observation.Result, observation.FinishedAt)
		item.Tool.Result = result
		return item, offload, false, nil
	}
	return item, nil, false, nil
}

const (
	inlineToolResultBytes = 64 << 10
	previewToolResultBytes = 16 << 10
)

func projectToolResult(sessionID, itemID, toolName, body string, createdAt time.Time) (any, *toolresult.Record) {
	if len(body) <= inlineToolResultBytes {
		return bestEffortJSON(body), nil
	}
	id := toolResultID(itemID)
	prefix := utf8Prefix(body, previewToolResultBytes)
	preview := prefix + fmt.Sprintf("\n\n[… %d bytes omitted. Continue with read_tool_result: {\"result_id\":%q}]", len(body)-len(prefix), id)
	record := &toolresult.Record{ID: id, SessionID: sessionID, ItemID: itemID, ToolName: toolName, Preview: preview, Body: body, CreatedAt: createdAt}
	return preview, record
}

func toolResultID(itemID string) string {
	digest := sha256.Sum256([]byte("tool-result\x00" + itemID))
	return "tr_" + hex.EncodeToString(digest[:16])
}

func utf8Prefix(value string, limit int) string {
	if len(value) <= limit { return value }
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) { end-- }
	return value[:end]
}

func bestEffortJSON(raw string) any {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil { return raw }
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) { return raw }
	return value
}

func stableItemID(runID, source string) string {
	digest := sha256.Sum256([]byte(runID + "\x00" + source))
	return "itm_" + hex.EncodeToString(digest[:16])
}

func mergeRunUsage(metrics *protocol.RunMetrics, usage protocol.Usage, steps int) {
	if metrics.Usage == nil { metrics.Usage = &protocol.Usage{} }
	metrics.Usage.InputTokens += usage.InputTokens
	metrics.Usage.OutputTokens += usage.OutputTokens
	metrics.Usage.CacheReadTokens += usage.CacheReadTokens
	metrics.Usage.CacheWriteTokens += usage.CacheWriteTokens
	metrics.Usage.ReasoningTokens += usage.ReasoningTokens
	mergeUsageCost(&metrics.Usage.CostUSD, usage.CostUSD)
	if len(usage.ByModel) > 0 && metrics.Usage.ByModel == nil { metrics.Usage.ByModel = make(map[string]protocol.ModelUsage) }
	for model, value := range usage.ByModel {
		current := metrics.Usage.ByModel[model]
		current.InputTokens += value.InputTokens
		current.OutputTokens += value.OutputTokens
		current.CacheReadTokens += value.CacheReadTokens
		current.CacheWriteTokens += value.CacheWriteTokens
		current.ReasoningTokens += value.ReasoningTokens
		mergeUsageCost(&current.CostUSD, value.CostUSD)
		metrics.Usage.ByModel[model] = current
	}
	metrics.Steps += steps
}

func mergeUsageCost(total **float64, value *float64) {
	if value == nil {
		return
	}
	merged := *value
	if *total != nil {
		merged += **total
	}
	*total = &merged
}
