package render

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/failure"
)

// NDJSON renders one event per line as a JSON object, so
// a consumer can read a run incrementally without waiting for it to end.
//
// The shape is the CLI's own output contract, not the runtime's wire format. A
// caller here wants what happened, already folded; the wire additionally carries
// segment identity, replay windows and dedup keys that exist to make a
// reconnecting consumer correct and would be noise in a pipe.
type NDJSON struct {
	enc       *json.Encoder
	err       error
	scope     runScope
	sessionID string
}

// NewNDJSON builds an event-stream renderer over w.
func NewNDJSON(w io.Writer) *NDJSON {
	return &NDJSON{enc: json.NewEncoder(w)}
}

// Begin binds the stream to the accepted run without emitting a synthetic
// event. The runtime's segment.started event remains the first output frame.
func (n *NDJSON) Begin(run agent.Run, _ agent.RunOptions) error {
	if n.err != nil {
		return n.err
	}
	if err := run.Validate(); err != nil {
		n.err = fmt.Errorf("begin NDJSON: %w", err)
		return n.err
	}
	if err := n.scope.bind(run); err != nil {
		n.err = fmt.Errorf("begin NDJSON: %w", err)
		return n.err
	}
	n.sessionID = run.SessionID
	return nil
}

// eventRecord is one output line.
//
// One struct with optional members, rather than a type per event: this is a wire
// shape, where a single discriminated object keeps a decoder's job simple and
// lets a reader ignore what it does not use. Type is always set and is the only
// field a consumer must switch on.
type eventRecord struct {
	Type             string            `json:"type"`
	EventID          string            `json:"eventId,omitzero"`
	SegmentID        string            `json:"segmentId,omitzero"`
	StreamSegmentID  string            `json:"streamSegmentId,omitzero"`
	Status           string            `json:"status,omitzero"`
	Revision         uint64            `json:"revision,omitzero"`
	At               time.Time         `json:"at,omitzero"`
	RunID            string            `json:"runId,omitzero"`
	SpawnedByBlockID string            `json:"spawnedByBlockId,omitzero"`
	ParentRunID      string            `json:"parentRunId,omitzero"`
	RootRunID        string            `json:"rootRunId,omitzero"`
	SessionID        string            `json:"sessionId,omitzero"`
	ItemID           string            `json:"itemId,omitzero"`
	Options          *runOptionsJSON   `json:"options,omitzero"`
	BlockID          string            `json:"blockId,omitzero"`
	Text             string            `json:"text,omitzero"`
	Index            *int              `json:"index,omitempty"`
	Step             *int              `json:"step,omitempty"`
	ContextTokens    *int64            `json:"contextTokens,omitempty"`
	Activity         string            `json:"activity,omitzero"`
	Name             string            `json:"name,omitzero"`
	Payload          json.RawMessage   `json:"payload,omitempty"`
	Block            *blockFrame       `json:"block,omitzero"`
	Transcript       []blockFrame      `json:"transcript,omitzero"`
	Runs             []runFrame        `json:"runs,omitzero"`
	Plan             []planFrame       `json:"plan,omitzero"`
	Interactions     []interactionJSON `json:"interactions,omitzero"`
	Outcome          *outcomeJSON      `json:"outcome,omitzero"`
	Usage            *usageJSON        `json:"usage,omitzero"`
}

type runOptionsJSON struct {
	Provider       string  `json:"provider,omitzero"`
	Model          string  `json:"model,omitzero"`
	MaxTotalTokens int64   `json:"maxTotalTokens,omitzero"`
	MaxSteps       int     `json:"maxSteps,omitzero"`
	MaxBudgetUSD   float64 `json:"maxBudgetUsd,omitzero"`
}

type blockFrame struct {
	ID              string            `json:"id"`
	RunID           string            `json:"runId,omitzero"`
	Status          string            `json:"status"`
	Kind            string            `json:"kind"`
	CreatedAt       time.Time         `json:"createdAt,omitzero"`
	Redacted        bool              `json:"redacted,omitzero"`
	DroppedMessages int               `json:"droppedMessages,omitzero"`
	Text            string            `json:"text,omitzero"`
	Attachments     []attachmentFrame `json:"attachments,omitzero"`
	Images          []imageFrame      `json:"images,omitzero"`
	Question        *interactionJSON  `json:"question,omitzero"`
	Tool            *toolFrame        `json:"tool,omitzero"`
}

type attachmentFrame struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	MimeType string `json:"mimeType,omitzero"`
	Size     int64  `json:"size"`
}

type imageFrame struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	Data     []byte `json:"data"`
	Size     int64  `json:"size"`
}

type toolFrame struct {
	Kind       string           `json:"kind"`
	Name       string           `json:"name"`
	Summary    string           `json:"summary,omitzero"`
	Status     string           `json:"status"`
	Safety     string           `json:"safetyClass,omitzero"`
	StartedAt  time.Time        `json:"startedAt,omitzero"`
	FinishedAt time.Time        `json:"finishedAt,omitzero"`
	Command    string           `json:"command,omitzero"`
	Path       string           `json:"path,omitzero"`
	Query      string           `json:"query,omitzero"`
	URL        string           `json:"url,omitzero"`
	Output     string           `json:"output,omitzero"`
	Arguments  json.RawMessage  `json:"arguments,omitempty"`
	Result     json.RawMessage  `json:"result,omitempty"`
	Problem    *failure.Problem `json:"problem,omitempty"`
	Diff       string           `json:"diff,omitzero"`
	ExitCode   *int             `json:"exitCode,omitzero"`
	DurationMS float64          `json:"durationMs,omitzero"`
}

type planFrame struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type interactionJSON struct {
	Kind         string              `json:"kind"`
	RunID        string              `json:"runId"`
	ItemID       string              `json:"itemId"`
	Title        string              `json:"title"`
	Detail       string              `json:"detail,omitzero"`
	Tool         *toolFrame          `json:"tool,omitzero"`
	Diff         string              `json:"diff,omitzero"`
	Risk         string              `json:"risk,omitzero"`
	RuleHint     string              `json:"ruleHint,omitzero"`
	Rememberable bool                `json:"rememberable,omitzero"`
	Fields       []questionFieldJSON `json:"fields,omitzero"`
	Answers      [][]string          `json:"answers,omitempty"`
}

type questionFieldJSON struct {
	Prompt      string               `json:"prompt"`
	Header      string               `json:"header,omitzero"`
	Kind        string               `json:"kind"`
	AllowCustom bool                 `json:"allowCustom,omitzero"`
	Options     []questionOptionJSON `json:"options,omitzero"`
}

type questionOptionJSON struct {
	Label       string `json:"label"`
	Description string `json:"description,omitzero"`
	Preview     string `json:"preview,omitzero"`
}

type outcomeJSON struct {
	Status  string           `json:"status"`
	Error   string           `json:"error,omitzero"`
	Problem *failure.Problem `json:"problem,omitempty"`
	Detail  string           `json:"detail,omitzero"`
}

type usageJSON struct {
	InputTokens      int64                     `json:"inputTokens"`
	OutputTokens     int64                     `json:"outputTokens"`
	CacheReadTokens  int64                     `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64                     `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64                     `json:"reasoningTokens,omitzero"`
	CostUSD          *float64                  `json:"costUsd,omitempty"`
	ByModel          map[string]modelUsageJSON `json:"byModel,omitempty"`
	Steps            int                       `json:"steps,omitzero"`
	DurationMS       float64                   `json:"durationMs,omitzero"`
}

type modelUsageJSON struct {
	InputTokens      int64    `json:"inputTokens"`
	OutputTokens     int64    `json:"outputTokens"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitzero"`
	CostUSD          *float64 `json:"costUsd,omitempty"`
}

// Render writes one line. As with [Text], the first error sticks.
func (n *NDJSON) Render(envelope agent.RunEvent) error {
	if n.err != nil {
		return n.err
	}
	if err := agent.ValidateEvent(envelope.Event); err != nil {
		n.err = fmt.Errorf("render NDJSON event: %w", err)
		return n.err
	}
	if err := n.scope.accept(envelope); err != nil {
		n.err = fmt.Errorf("render NDJSON event: %w", err)
		return n.err
	}
	f, err := encodeEventFrame(envelope)
	if err != nil {
		n.err = err
		return n.err
	}
	f.EventID, f.SegmentID, f.StreamSegmentID, f.At = envelope.EventID, envelope.SegmentID, envelope.StreamSegment(), envelope.At
	if f.RunID == "" {
		f.RunID = envelope.RunID
	}
	n.err = n.enc.Encode(f)
	return n.err
}

// Reconcile emits a replacement snapshot frame. Unlike a runtime event it has
// no eventId: consumers replace their durable projection with this frame, then
// continue folding later segment events on top.
func (n *NDJSON) Reconcile(snapshot agent.SessionSnapshot) error {
	if n.err != nil {
		return n.err
	}
	if err := snapshot.Validate(); err != nil {
		n.err = fmt.Errorf("render NDJSON snapshot: %w", err)
		return n.err
	}
	target, err := resolveSnapshotRun(snapshot, n.scope.rootID)
	if err != nil {
		n.err = fmt.Errorf("render NDJSON snapshot: %w", err)
		return n.err
	}
	if err := n.scope.restore(snapshot, target.ID); err != nil {
		n.err = fmt.Errorf("render NDJSON snapshot: %w", err)
		return n.err
	}
	n.sessionID = snapshot.Session.ID
	frame := eventRecord{
		Type: "run.snapshot", RunID: target.ID, SessionID: snapshot.Session.ID, Status: string(target.Status),
		Transcript: make([]blockFrame, 0, len(snapshot.Transcript)),
		Runs:       make([]runFrame, 0, len(snapshot.Runs)),
	}
	for _, run := range snapshot.Runs {
		if run.ID == target.ID || run.Lineage.RootRunID == target.ID {
			frame.Runs = append(frame.Runs, encodeRun(run))
		}
	}
	for _, block := range snapshot.Transcript {
		if n.scope.contains(block.RunID) {
			frame.Transcript = append(frame.Transcript, *encodeBlock(block))
		}
	}
	if latest, ok := snapshot.LatestRun(); ok && latest.ID == target.ID {
		frame.Revision, frame.Plan = snapshot.PlanRevision, encodePlan(snapshot.Plan)
	}
	if target.Status == agent.RunStatusWaiting {
		frame.Interactions = encodeInteractions(snapshot.Interactions)
	}
	if target.Status == agent.RunStatusFinished {
		finished := encodeFinishedFrame(agent.RunFinished{Outcome: target.Outcome, Usage: target.Usage})
		frame.Outcome, frame.Usage = finished.Outcome, finished.Usage
	}
	n.err = n.enc.Encode(frame)
	return n.err
}

func encodeEventFrame(envelope agent.RunEvent) (eventRecord, error) {
	switch event := envelope.Event.(type) {
	case agent.SegmentStarted:
		return eventRecord{
			Type: "segment.started", RunID: event.Run.ID, SessionID: event.Run.SessionID,
			SpawnedByBlockID: event.Run.Lineage.SpawnedByBlockID,
			ParentRunID:      event.Run.Lineage.ParentRunID, RootRunID: event.Run.Lineage.RootRunID,
		}, nil
	case agent.BlockStarted:
		return eventRecord{Type: "block.started", Block: encodeBlock(event.Block)}, nil
	case agent.BlockDelta:
		return eventRecord{
			Type: "block.delta", BlockID: event.BlockID, Text: event.Text, Index: event.ContentIndex,
		}, nil
	case agent.ToolArgumentsDelta:
		return eventRecord{Type: "tool.arguments.delta", BlockID: event.BlockID, Text: event.Text}, nil
	case agent.RunProgress:
		frame := eventRecord{
			Type: "run.progress", Step: event.Step, ContextTokens: event.ContextTokens, Activity: event.Activity,
		}
		if event.Usage != nil {
			frame.Usage = encodeUsage(*event.Usage)
		}
		return frame, nil
	case agent.CustomEvent:
		return eventRecord{Type: "custom", Name: event.Name, Payload: json.RawMessage(event.PayloadJSON)}, nil
	case agent.BlockCompleted:
		return eventRecord{Type: "block.completed", Block: encodeBlock(event.Block)}, nil
	case agent.PlanChanged:
		return eventRecord{Type: "plan.changed", Revision: event.Revision, Plan: encodePlan(event.Items)}, nil
	case agent.RunInterrupted:
		return eventRecord{Type: "run.interrupted", Interactions: encodeInteractions(event.Interactions), Usage: encodeUsage(event.Usage)}, nil
	case agent.RunSuspended:
		return eventRecord{Type: "run.suspended", Usage: encodeUsage(event.Usage)}, nil
	case agent.RunFinished:
		return encodeFinishedFrame(event), nil
	default:
		return eventRecord{}, fmt.Errorf("render NDJSON event: unsupported event %T", envelope.Event)
	}
}

func encodeFinishedFrame(event agent.RunFinished) eventRecord {
	return eventRecord{
		Type:    "run.finished",
		Outcome: encodeOutcome(event.Outcome),
		Usage:   encodeUsage(event.Usage),
	}
}

func encodeOutcome(outcome agent.Outcome) *outcomeJSON {
	errorText := ""
	if outcome.Problem != nil {
		errorText = outcome.Problem.Message("")
	}
	return &outcomeJSON{
		Status: string(outcome.Status), Error: errorText,
		Problem: outcome.Problem.Clone(), Detail: outcome.Detail,
	}
}

func encodeUsage(usage agent.Usage) *usageJSON {
	encoded := &usageJSON{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens:  usage.ReasoningTokens,
		CostUSD:          cloneFloat64(usage.CostUSD),
		Steps:            usage.Steps,
		DurationMS:       float64(usage.Duration.Milliseconds()),
	}
	if usage.ByModel != nil {
		encoded.ByModel = make(map[string]modelUsageJSON, len(usage.ByModel))
		for model, value := range usage.ByModel {
			encoded.ByModel[model] = modelUsageJSON{
				InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
				CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
				ReasoningTokens: value.ReasoningTokens, CostUSD: cloneFloat64(value.CostUSD),
			}
		}
	}
	return encoded
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func encodeRunOptions(options agent.RunOptions) *runOptionsJSON {
	return &runOptionsJSON{
		Provider: options.Provider, Model: options.Model,
		MaxTotalTokens: options.Limits.MaxTotalTokens,
		MaxSteps:       options.Limits.MaxSteps,
		MaxBudgetUSD:   options.Limits.MaxBudgetUSD,
	}
}

func encodeInteractions(interactions []agent.Interaction) []interactionJSON {
	out := make([]interactionJSON, 0, len(interactions))
	for _, interaction := range interactions {
		if encoded := encodeInteraction(interaction); encoded != nil {
			out = append(out, *encoded)
		}
	}
	return out
}

func encodeInteraction(interaction agent.Interaction) *interactionJSON {
	switch item := interaction.(type) {
	case agent.Approval:
		return &interactionJSON{
			Kind: "approval", RunID: item.RunID, ItemID: item.ItemID, Title: item.Title,
			Detail: item.Detail, Tool: encodeTool(item.Tool), Diff: item.Diff, Risk: string(item.Risk),
			RuleHint: item.RuleHint, Rememberable: item.Rememberable,
		}
	case agent.Question:
		out := &interactionJSON{
			Kind: "question", RunID: item.RunID, ItemID: item.ItemID,
			Title: item.Title, Detail: item.Detail, Answers: cloneStringMatrix(item.Answers),
		}
		for _, field := range item.Fields {
			encoded := questionFieldJSON{
				Prompt: field.Prompt, Header: field.Header,
				Kind: string(field.Kind), AllowCustom: field.AllowCustom,
			}
			for _, option := range field.Options {
				encoded.Options = append(encoded.Options, questionOptionJSON{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				})
			}
			out.Fields = append(out.Fields, encoded)
		}
		return out
	default:
		return nil
	}
}

func cloneStringMatrix(values [][]string) [][]string {
	if values == nil {
		return nil
	}
	cloned := make([][]string, len(values))
	for index, row := range values {
		cloned[index] = slices.Clone(row)
	}
	return cloned
}

// Close reports the first write error, if any. There is nothing to flush: a line
// is encoded per event.
func (n *NDJSON) Close() error { return n.err }

func encodeBlock(b agent.Block) *blockFrame {
	out := &blockFrame{
		ID: b.ID, RunID: b.RunID, Status: string(b.Status), Kind: string(b.Kind),
		CreatedAt: b.CreatedAt, Redacted: b.Redacted, DroppedMessages: b.DroppedMessages, Text: b.Text,
	}
	for _, attachment := range b.Attachments {
		out.Attachments = append(out.Attachments, attachmentFrame{
			ID: attachment.ID, Kind: string(attachment.Kind), Name: attachment.Name,
			Path: attachment.Path, MimeType: attachment.MimeType, Size: attachment.Size,
		})
	}
	for _, image := range b.Images {
		out.Images = append(out.Images, imageFrame{
			ID: image.ID, Name: image.Name, MIMEType: image.MIMEType,
			Data: image.Data, Size: int64(len(image.Data)),
		})
	}
	if b.Tool != nil {
		out.Tool = encodeTool(b.Tool)
	}
	if b.Question != nil {
		out.Question = encodeInteraction(*b.Question)
	}
	return out
}

func encodeTool(tool *agent.ToolCall) *toolFrame {
	if tool == nil {
		return nil
	}
	return &toolFrame{
		Kind:       string(tool.Kind),
		Name:       tool.Name,
		Summary:    tool.Summary,
		Status:     string(tool.Status),
		Safety:     string(tool.Safety),
		StartedAt:  tool.StartedAt,
		FinishedAt: tool.FinishedAt,
		Command:    tool.Command,
		Path:       tool.Path,
		Query:      tool.Query,
		URL:        tool.URL,
		Output:     tool.Output,
		Arguments:  json.RawMessage(tool.ArgumentsJSON),
		Result:     json.RawMessage(tool.ResultJSON),
		Problem:    tool.Problem.Clone(),
		Diff:       tool.Diff,
		ExitCode:   tool.ExitCode,
		DurationMS: float64(tool.Duration.Milliseconds()),
	}
}

func encodePlan(items []agent.PlanItem) []planFrame {
	out := make([]planFrame, 0, len(items))
	for _, it := range items {
		out = append(out, planFrame{Title: it.Title, Status: string(it.Status)})
	}
	return out
}
