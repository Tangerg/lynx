package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// ResultJSON folds a streamed run into one final JSON object. It retains only
// assistant prose and terminal metadata; callers that need every event use [NDJSON].
type ResultJSON struct {
	enc     *json.Encoder
	err     error
	closed  bool
	started bool
	frame   resultFrame
	live    map[string]*strings.Builder
	prose   []string
}

type resultFrame struct {
	Type        string           `json:"type"`
	Status      string           `json:"status"`
	RunID       string           `json:"runId"`
	SessionID   string           `json:"sessionId"`
	Text        string           `json:"text,omitzero"`
	Options     *runOptionsJSON  `json:"options,omitzero"`
	Interaction *interactionJSON `json:"interaction,omitzero"`
	Outcome     *outcomeJSON     `json:"outcome,omitzero"`
	Usage       *usageJSON       `json:"usage,omitzero"`
}

// NewResultJSON builds a renderer that emits at most one JSON result from Close.
func NewResultJSON(w io.Writer) *ResultJSON {
	return &ResultJSON{enc: json.NewEncoder(w), live: make(map[string]*strings.Builder)}
}

// Begin records the accepted run before its first subscription opens, so a
// transport failure can still produce a useful incomplete result.
func (r *ResultJSON) Begin(run agent.Run, options agent.RunOptions) error {
	if r.err != nil {
		return r.err
	}
	if r.closed {
		r.err = errors.New("begin result after close")
		return r.err
	}
	if err := run.Validate(); err != nil {
		r.err = fmt.Errorf("begin result: %w", err)
		return r.err
	}
	r.started = true
	r.frame = resultFrame{
		Type: "result", Status: "incomplete", RunID: run.ID,
		SessionID: run.SessionID, Options: encodeRunOptions(options),
	}
	return nil
}

// Render folds one validated event into the final result.
func (r *ResultJSON) Render(envelope agent.Envelope) error {
	if r.err != nil {
		return r.err
	}
	if r.closed {
		r.err = errors.New("render result after close")
		return r.err
	}
	if err := agent.ValidateEvent(envelope.Event); err != nil {
		r.err = fmt.Errorf("render result event: %w", err)
		return r.err
	}
	r.fold(envelope)
	return nil
}

func (r *ResultJSON) fold(envelope agent.Envelope) {
	switch event := envelope.Event.(type) {
	case agent.RunStarted:
		if !r.started {
			r.started = true
			r.frame = resultFrame{Type: "result", Status: "incomplete"}
		}
		r.frame.RunID, r.frame.SessionID = event.RunID, event.SessionID
		r.frame.Options = encodeRunOptions(event.Options)
		if r.frame.RunID == "" {
			r.frame.RunID = envelope.RunID
		}
		if r.frame.SessionID == "" {
			r.frame.SessionID = envelope.SessionID
		}
	case agent.BlockStarted:
		r.begin(event.Block)
	case agent.BlockDelta:
		if body := r.live[event.BlockID]; body != nil {
			body.WriteString(event.Text)
		}
	case agent.BlockCompleted:
		r.complete(event.Block)
	case agent.RunInterrupted:
		r.frame.Status = "interrupted"
		r.frame.Interaction = encodeInteraction(event.Interaction)
	case agent.RunResumed:
		r.frame.Status = "incomplete"
		r.frame.Interaction = nil
	case agent.RunFinished:
		r.frame.Status = string(event.Outcome.Status)
		r.frame.Interaction = nil
		finished := encodeFinishedFrame(event)
		r.frame.Outcome, r.frame.Usage = finished.Outcome, finished.Usage
	case agent.PlanChanged:
		// A final result intentionally omits incremental plan state.
	}
}

func (r *ResultJSON) begin(block agent.Block) {
	if block.Kind != agent.BlockAssistant {
		return
	}
	body := new(strings.Builder)
	body.WriteString(block.Text)
	r.live[block.ID] = body
}

func (r *ResultJSON) complete(block agent.Block) {
	if block.Kind != agent.BlockAssistant {
		return
	}
	text := block.Text
	if text == "" {
		if body := r.live[block.ID]; body != nil {
			text = body.String()
		}
	}
	delete(r.live, block.ID)
	if text != "" {
		r.prose = append(r.prose, text)
	}
}

// Close emits one object after at least a run.started event. It is idempotent;
// failures before a run starts leave stdout empty.
func (r *ResultJSON) Close() error {
	if r.closed {
		return r.err
	}
	r.closed = true
	if r.err != nil || !r.started {
		return r.err
	}
	r.frame.Text = strings.Join(r.prose, "\n\n")
	r.err = r.enc.Encode(r.frame)
	return r.err
}
