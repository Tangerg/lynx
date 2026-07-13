package runs

import (
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// reduction is one canonical output plus the durable fact and live nudge that
// arise from the same EngineEvent decision. The pump commits it before placing
// Event on the Journal.
type reduction struct {
	Event     RunEvent
	Commit    *EventCommit
	Nudge     *Nudge
	Interrupt bool
}

type reducerConfig struct {
	RunID        string
	SegmentID    string
	SessionID    string
	Cwd          string
	TurnID       string
	Provider     string
	Model        string
	CreatedAt    time.Time
	UserInput    []ContentBlock
	Pending      *interrupts.Pending
	Now          func() time.Time
	CancelReason func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical RunEvent family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg       reducerConfig
	resume    *resumeBinding
	itemSeq   int
	step      int
	userInput []ContentBlock
	text      *openText
	reasoning *openText
	tools     map[string]*openTool
	drained   []interrupts.DrainedTool
	errMsg    string
	errCode   string
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	id          string
	createdAt   time.Time
	name        string
	args        string
	safetyClass string
}

func newReducer(cfg reducerConfig) *reducer {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cfg.Now = now
	var resume *resumeBinding
	if cfg.Pending != nil {
		resume = resumeBindingFrom(*cfg.Pending)
	}
	return &reducer{
		cfg: cfg, resume: resume, userInput: cfg.UserInput,
		tools: map[string]*openTool{},
	}
}

func (r *reducer) nextItemID() string {
	r.itemSeq++
	return "item_" + r.cfg.SegmentID + "_" + strconv.Itoa(r.itemSeq)
}

func userMessageItemID(segmentID string) string { return "item_" + segmentID + "_u" }

func (r *reducer) open() []reduction {
	createdAt := r.cfg.CreatedAt
	if createdAt.IsZero() {
		createdAt = r.now()
	}
	out := []RunEvent{SegmentStarted{Run: transcript.Run{
		ID: r.cfg.RunID, SessionID: r.cfg.SessionID,
		Provider: r.cfg.Provider, Model: r.cfg.Model,
		State: execution.Running, CreatedAt: createdAt, UpdatedAt: r.now(), MessageMark: -1,
	}}}
	out = append(out, r.openUserMessage()...)
	out = append(out, r.resumeQuestionCompletions()...)
	return r.project(out)
}

func (r *reducer) reduce(ev EngineEvent) []reduction {
	var out []RunEvent
	switch e := ev.(type) {
	case TurnStart:
		return nil
	case MessageDelta:
		out = r.closeReasoning()
		out = append(out, r.appendText(e.Text)...)
	case ReasoningDelta:
		out = r.closeText()
		out = append(out, r.appendReasoning(e.Text)...)
	case ToolCallStart:
		out = r.toolStart(e)
	case ToolCallEnd:
		out = r.toolEnd(e)
	case UsageReported:
		out = r.usageProgress(e)
	case SteerMessage:
		out = r.steerMessage(e)
	case TodosUpdated:
		out = r.todosSnapshot(e)
	case ErrorEvent:
		r.errMsg, r.errCode = e.Message, e.Code
		return nil
	case CompactBoundary:
		out = r.compaction(e)
	case MemoryUpdated:
		return nil
	case TurnInterrupted:
		out = r.interrupt(e)
	case TurnEnd:
		out = r.turnEnd(e)
	default:
		panic("runs: unhandled engine event")
	}
	return r.project(out)
}

func (r *reducer) synthesizeTerminal() []reduction {
	out := r.closeStreaming()
	out = append(out, r.drainTools()...)
	result := &RunResult{}
	outcome := execution.OutcomeCanceled
	if r.errMsg != "" {
		outcome = execution.OutcomeError
		result.Error = r.classifyRunError(r.errMsg)
	}
	detail := ""
	if outcome == execution.OutcomeCanceled && r.cfg.CancelReason != nil {
		detail = r.cfg.CancelReason()
	}
	out = append(out, r.finishedRun(outcome, result, detail))
	return r.project(out)
}

func (r *reducer) abort(msg string) { r.errMsg = msg }

func (r *reducer) project(events []RunEvent) []reduction {
	out := make([]reduction, 0, len(events))
	for _, event := range events {
		out = append(out, r.projectOne(event))
	}
	return out
}

func (r *reducer) projectOne(event RunEvent) reduction {
	commit := EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		e.Item.SessionID = r.cfg.SessionID
		event = e
		commit.Item = &e.Item
		if paths := fileChangedPaths(e.Item); len(paths) > 0 {
			nudge = &Nudge{Cwd: r.cfg.Cwd, Paths: paths}
		}
	case SegmentStarted:
		commit.Run = &e.Run
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State == execution.Interrupted {
			commit.Interrupt = &interrupts.Pending{
				RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, TurnID: r.cfg.TurnID,
				Provider: r.cfg.Provider, Model: r.cfg.Model,
				Interrupts: e.Run.Interrupts, DrainedTools: r.drained,
				RunCreatedAt: r.cfg.CreatedAt, CreatedAt: r.now(),
			}
			commit.State = StateSuspend
			return reduction{Event: event, Commit: &commit, Interrupt: true}
		}
		commit.State = StateTerminalize
		if e.Run.Outcome != nil {
			commit.Outcome = *e.Run.Outcome
		}
	}
	return reduction{Event: event, Commit: commitOrNil(commit), Nudge: nudge}
}

func commitOrNil(commit EventCommit) *EventCommit {
	if commit.Item == nil && commit.Run == nil && commit.Interrupt == nil && commit.State == StateUnchanged {
		return nil
	}
	return &commit
}

func (r *reducer) now() time.Time { return r.cfg.Now().UTC() }

func fileChangedPaths(item Item) []string {
	if item.Kind != ToolCall || item.Status != ItemSucceeded || item.Error != nil || item.Tool == nil {
		return nil
	}
	switch strings.ToLower(item.Tool.Name) {
	case "write", "edit":
		if path, _ := item.Tool.Arguments["file_path"].(string); path != "" {
			return []string{path}
		}
	}
	return nil
}
