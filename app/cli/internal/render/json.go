package render

import (
	"encoding/json"
	"io"

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
	Type      string        `json:"type"`
	RunID     string        `json:"runId,omitempty"`
	SessionID string        `json:"sessionId,omitempty"`
	BlockID   string        `json:"blockId,omitempty"`
	Text      string        `json:"text,omitempty"`
	Block     *blockFrame   `json:"block,omitempty"`
	Plan      []planFrame   `json:"plan,omitempty"`
	Approval  *approvalJSON `json:"approval,omitempty"`
	Outcome   *outcomeJSON  `json:"outcome,omitempty"`
	Usage     *usageJSON    `json:"usage,omitempty"`
}

type blockFrame struct {
	ID   string     `json:"id"`
	Kind string     `json:"kind"`
	Text string     `json:"text,omitempty"`
	Tool *toolFrame `json:"tool,omitempty"`
}

type toolFrame struct {
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Status     string  `json:"status"`
	Output     string  `json:"output,omitempty"`
	Diff       string  `json:"diff,omitempty"`
	DurationMS float64 `json:"durationMs,omitempty"`
}

type planFrame struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type approvalJSON struct {
	InterruptID string `json:"interruptId"`
	Title       string `json:"title"`
	Detail      string `json:"detail,omitempty"`
	Diff        string `json:"diff,omitempty"`
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
func (j *JSON) Render(ev client.Event) error {
	if j.err != nil {
		return j.err
	}
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
	case client.RunParked:
		f = frame{Type: "run.parked", Approval: &approvalJSON{
			InterruptID: e.Approval.InterruptID,
			Title:       e.Approval.Title,
			Detail:      e.Approval.Detail,
			Diff:        e.Approval.Diff,
		}}
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
	j.err = j.enc.Encode(f)
	return j.err
}

// Close reports the first write error, if any. There is nothing to flush: a line
// is encoded per event.
func (j *JSON) Close() error { return j.err }

func encodeBlock(b client.Block) *blockFrame {
	out := &blockFrame{ID: b.ID, Kind: string(b.Kind), Text: b.Text}
	if b.Tool != nil {
		out.Tool = &toolFrame{
			Name:       b.Tool.Name,
			Summary:    b.Tool.Summary,
			Status:     string(b.Tool.Status),
			Output:     b.Tool.Output,
			Diff:       b.Tool.Diff,
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
