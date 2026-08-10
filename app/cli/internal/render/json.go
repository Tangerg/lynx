package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// JSON renders one event per line as a JSON object — newline-delimited JSON, so
// a consumer can read a run incrementally without waiting for it to end.
//
// The shape is the CLI's own output contract, not the runtime's wire format. A
// caller here wants what happened, already folded; the wire additionally carries
// segment identity, replay windows and dedup keys that exist to make a
// reconnecting consumer correct and would be noise in a pipe.
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
	Cursor      agent.Cursor     `json:"cursor"`
	At          time.Time        `json:"at,omitzero"`
	RunID       string           `json:"runId,omitzero"`
	SessionID   string           `json:"sessionId,omitzero"`
	InterruptID string           `json:"interruptId,omitzero"`
	Options     *runOptionsJSON  `json:"options,omitzero"`
	BlockID     string           `json:"blockId,omitzero"`
	Text        string           `json:"text,omitzero"`
	Block       *blockFrame      `json:"block,omitzero"`
	Plan        []planFrame      `json:"plan,omitzero"`
	Interaction *interactionJSON `json:"interaction,omitzero"`
	Outcome     *outcomeJSON     `json:"outcome,omitzero"`
	Usage       *usageJSON       `json:"usage,omitzero"`
}

type runOptionsJSON struct {
	Model      string `json:"model"`
	Mode       string `json:"mode"`
	Permission string `json:"permission"`
	Effort     string `json:"effort"`
}

type blockFrame struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Text        string            `json:"text,omitzero"`
	Attachments []attachmentFrame `json:"attachments,omitzero"`
	Tool        *toolFrame        `json:"tool,omitzero"`
}

type attachmentFrame struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	MimeType string `json:"mimeType,omitzero"`
	Size     int64  `json:"size"`
}

type toolFrame struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitzero"`
	Status     string  `json:"status"`
	Command    string  `json:"command,omitzero"`
	Path       string  `json:"path,omitzero"`
	Query      string  `json:"query,omitzero"`
	URL        string  `json:"url,omitzero"`
	Output     string  `json:"output,omitzero"`
	Diff       string  `json:"diff,omitzero"`
	ExitCode   *int    `json:"exitCode,omitzero"`
	DurationMS float64 `json:"durationMs,omitzero"`
}

type planFrame struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type interactionJSON struct {
	Kind        string              `json:"kind"`
	InterruptID string              `json:"interruptId"`
	Title       string              `json:"title"`
	Detail      string              `json:"detail,omitzero"`
	Diff        string              `json:"diff,omitzero"`
	Risk        string              `json:"risk,omitzero"`
	RuleHint    string              `json:"ruleHint,omitzero"`
	Fields      []questionFieldJSON `json:"fields,omitzero"`
}

type questionFieldJSON struct {
	ID          string               `json:"id"`
	Label       string               `json:"label"`
	Description string               `json:"description,omitzero"`
	Kind        string               `json:"kind"`
	Required    bool                 `json:"required,omitzero"`
	Placeholder string               `json:"placeholder,omitzero"`
	Options     []questionOptionJSON `json:"options,omitzero"`
}

type questionOptionJSON struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitzero"`
	Recommended bool   `json:"recommended,omitzero"`
}

type outcomeJSON struct {
	Status string `json:"status"`
	Error  string `json:"error,omitzero"`
}

type usageJSON struct {
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CachedTokens int64   `json:"cachedTokens,omitzero"`
	CostUSD      float64 `json:"costUsd,omitzero"`
	DurationMS   float64 `json:"durationMs,omitzero"`
}

// Render writes one line. As with [Text], the first error sticks.
func (j *JSON) Render(envelope agent.Envelope) error {
	if j.err != nil {
		return j.err
	}
	if err := agent.ValidateEvent(envelope.Event); err != nil {
		j.err = fmt.Errorf("render JSON event: %w", err)
		return j.err
	}
	f, err := encodeEventFrame(envelope)
	if err != nil {
		j.err = err
		return j.err
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

func encodeEventFrame(envelope agent.Envelope) (frame, error) {
	switch event := envelope.Event.(type) {
	case agent.RunStarted:
		return frame{Type: "run.started", RunID: event.RunID, SessionID: event.SessionID, Options: encodeRunOptions(event.Options)}, nil
	case agent.BlockStarted:
		return frame{Type: "block.started", Block: encodeBlock(event.Block)}, nil
	case agent.BlockDelta:
		return frame{Type: "block.delta", BlockID: event.BlockID, Text: event.Text}, nil
	case agent.BlockCompleted:
		return frame{Type: "block.completed", Block: encodeBlock(event.Block)}, nil
	case agent.PlanChanged:
		return frame{Type: "plan.changed", Plan: encodePlan(event.Items)}, nil
	case agent.RunResumed:
		return frame{Type: "run.resumed", RunID: envelope.RunID, InterruptID: event.InterruptID}, nil
	case agent.RunInterrupted:
		return frame{Type: "run.interrupted", Interaction: encodeInteraction(event.Interaction)}, nil
	case agent.RunFinished:
		return encodeFinishedFrame(event), nil
	default:
		return frame{}, fmt.Errorf("render JSON event: unsupported event %T", envelope.Event)
	}
}

func encodeFinishedFrame(event agent.RunFinished) frame {
	return frame{
		Type:    "run.finished",
		Outcome: &outcomeJSON{Status: string(event.Outcome.Status), Error: event.Outcome.Error},
		Usage: &usageJSON{
			InputTokens:  event.Usage.InputTokens,
			OutputTokens: event.Usage.OutputTokens,
			CachedTokens: event.Usage.CachedTokens,
			CostUSD:      event.Usage.CostUSD,
			DurationMS:   float64(event.Usage.Duration.Milliseconds()),
		},
	}
}

func encodeRunOptions(options agent.RunOptions) *runOptionsJSON {
	return &runOptionsJSON{
		Model: options.Model, Mode: string(options.Mode),
		Permission: string(options.Permission), Effort: options.Effort,
	}
}

func encodeInteraction(interaction agent.Interaction) *interactionJSON {
	switch item := interaction.(type) {
	case agent.Approval:
		return &interactionJSON{
			Kind: "approval", InterruptID: item.InterruptID, Title: item.Title,
			Detail: item.Detail, Diff: item.Diff, Risk: item.Risk, RuleHint: item.RuleHint,
		}
	case agent.Question:
		out := &interactionJSON{Kind: "question", InterruptID: item.InterruptID, Title: item.Title, Detail: item.Detail}
		for _, field := range item.Fields {
			encoded := questionFieldJSON{
				ID: field.ID, Label: field.Label, Description: field.Description,
				Kind: string(field.Kind), Required: field.Required, Placeholder: field.Placeholder,
			}
			for _, option := range field.Options {
				encoded.Options = append(encoded.Options, questionOptionJSON{
					Value: option.Value, Label: option.Label,
					Description: option.Description, Recommended: option.Recommended,
				})
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

func encodeBlock(b agent.Block) *blockFrame {
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

func encodePlan(items []agent.PlanItem) []planFrame {
	out := make([]planFrame, 0, len(items))
	for _, it := range items {
		out = append(out, planFrame{Title: it.Title, Status: string(it.Status)})
	}
	return out
}
