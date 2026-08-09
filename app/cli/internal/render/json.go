package render

import (
	"encoding/json"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// JSON renders one event per line as a JSON object — newline-delimited JSON, so
// a consumer can read a run incrementally without waiting for it to end.
//
// The shape is the CLI's own output contract, not the runtime's wire format. A
// caller here wants what happened, already folded; the wire additionally carries
// segment identity, replay windows and dedup keys that exist to make a
// reconnecting client correct and would be noise in a pipe.
type JSON struct {
	enc *json.Encoder
	err error
}

// NewJSON builds an NDJSON renderer over w.
func NewJSON(w io.Writer) *JSON {
	return &JSON{enc: json.NewEncoder(w)}
}

// frame is one output line.
//
// One struct with optional members, rather than a type per event: this is a wire
// shape, where a single discriminated object keeps a decoder's job simple and
// lets a reader ignore what it does not use. Type is always set and is the only
// field a consumer must switch on.
type frame struct {
	Type        string           `json:"type"`
	EventID     string           `json:"eventId"`
	Cursor      client.Cursor    `json:"cursor"`
	At          time.Time        `json:"at,omitzero"`
	RunID       string           `json:"runId,omitempty"`
	SessionID   string           `json:"sessionId,omitempty"`
	BlockID     string           `json:"blockId,omitempty"`
	Text        string           `json:"text,omitempty"`
	Block       *blockFrame      `json:"block,omitempty"`
	Plan        []planFrame      `json:"plan,omitempty"`
	Interaction *interactionJSON `json:"interaction,omitempty"`
	Outcome     *outcomeJSON     `json:"outcome,omitempty"`
	Usage       *usageJSON       `json:"usage,omitempty"`
}

type blockFrame struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Text        string            `json:"text,omitempty"`
	Attachments []attachmentFrame `json:"attachments,omitempty"`
	Tool        *toolFrame        `json:"tool,omitempty"`
}

type attachmentFrame struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	MimeType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size"`
}

type toolFrame struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Status     string  `json:"status"`
	Command    string  `json:"command,omitempty"`
	Path       string  `json:"path,omitempty"`
	Query      string  `json:"query,omitempty"`
	URL        string  `json:"url,omitempty"`
	Output     string  `json:"output,omitempty"`
	Diff       string  `json:"diff,omitempty"`
	ExitCode   *int    `json:"exitCode,omitempty"`
	DurationMS float64 `json:"durationMs,omitempty"`
}

type planFrame struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type interactionJSON struct {
	Kind        string              `json:"kind"`
	InterruptID string              `json:"interruptId"`
	Title       string              `json:"title"`
	Detail      string              `json:"detail,omitempty"`
	Diff        string              `json:"diff,omitempty"`
	Risk        string              `json:"risk,omitempty"`
	Fields      []questionFieldJSON `json:"fields,omitempty"`
}

type questionFieldJSON struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
}

type outcomeJSON struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type usageJSON struct {
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CachedTokens int64   `json:"cachedTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
	DurationMS   float64 `json:"durationMs,omitempty"`
}

// Render writes one line. As with [Text], the first error sticks.
func (j *JSON) Render(envelope client.Envelope) error {
	if j.err != nil {
		return j.err
	}
	ev := envelope.Event
	var f frame
	switch e := ev.(type) {
	case client.RunStarted:
		f = frame{Type: "run.started", RunID: e.RunID, SessionID: e.SessionID}
	case client.BlockStarted:
		f = frame{Type: "block.started", Block: encodeBlock(e.Block)}
	case client.BlockDelta:
		f = frame{Type: "block.delta", BlockID: e.BlockID, Text: e.Text}
	case client.BlockCompleted:
		f = frame{Type: "block.completed", Block: encodeBlock(e.Block)}
	case client.PlanChanged:
		f = frame{Type: "plan.changed", Plan: encodePlan(e.Items)}
	case client.RunResumed:
		f = frame{Type: "run.resumed", RunID: envelope.RunID}
	case client.RunInterrupted:
		f = frame{Type: "run.interrupted", Interaction: encodeInteraction(e.Interaction)}
	case client.RunFinished:
		f = frame{
			Type:    "run.finished",
			Outcome: &outcomeJSON{Status: string(e.Outcome.Status), Error: e.Outcome.Error},
			Usage: &usageJSON{
				InputTokens:  e.Usage.InputTokens,
				OutputTokens: e.Usage.OutputTokens,
				CachedTokens: e.Usage.CachedTokens,
				CostUSD:      e.Usage.CostUSD,
				DurationMS:   float64(e.Usage.Duration.Milliseconds()),
			},
		}
	}
	f.EventID, f.Cursor, f.At = envelope.ID, envelope.Cursor, envelope.At
	if f.RunID == "" {
		f.RunID = envelope.RunID
	}
	if f.SessionID == "" {
		f.SessionID = envelope.SessionID
	}
	j.err = j.enc.Encode(f)
	return j.err
}

func encodeInteraction(interaction client.Interaction) *interactionJSON {
	switch item := interaction.(type) {
	case client.Approval:
		return &interactionJSON{Kind: "approval", InterruptID: item.InterruptID, Title: item.Title, Detail: item.Detail, Diff: item.Diff, Risk: item.Risk}
	case client.Question:
		out := &interactionJSON{Kind: "question", InterruptID: item.InterruptID, Title: item.Title, Detail: item.Detail}
		for _, field := range item.Fields {
			encoded := questionFieldJSON{ID: field.ID, Label: field.Label, Kind: string(field.Kind), Required: field.Required}
			for _, option := range field.Options {
				encoded.Options = append(encoded.Options, option.Value)
			}
			out.Fields = append(out.Fields, encoded)
		}
		return out
	default:
		return nil
	}
}

// Close reports the first write error, if any. There is nothing to flush: a line
// is encoded per event.
func (j *JSON) Close() error { return j.err }

func encodeBlock(b client.Block) *blockFrame {
	out := &blockFrame{ID: b.ID, Kind: string(b.Kind), Text: b.Text}
	for _, attachment := range b.Attachments {
		out.Attachments = append(out.Attachments, attachmentFrame{
			ID: attachment.ID, Kind: string(attachment.Kind), Name: attachment.Name,
			Path: attachment.Path, MimeType: attachment.MimeType, Size: attachment.Size,
		})
	}
	if b.Tool != nil {
		out.Tool = &toolFrame{
			Kind:       string(b.Tool.Kind),
			Name:       b.Tool.Name,
			Summary:    b.Tool.Summary,
			Status:     string(b.Tool.Status),
			Command:    b.Tool.Command,
			Path:       b.Tool.Path,
			Query:      b.Tool.Query,
			URL:        b.Tool.URL,
			Output:     b.Tool.Output,
			Diff:       b.Tool.Diff,
			ExitCode:   b.Tool.ExitCode,
			DurationMS: float64(b.Tool.Duration.Milliseconds()),
		}
	}
	return out
}

func encodePlan(items []client.PlanItem) []planFrame {
	out := make([]planFrame, 0, len(items))
	for _, it := range items {
		out = append(out, planFrame{Title: it.Title, Status: string(it.Status)})
	}
	return out
}
