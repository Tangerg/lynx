package toolloop

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// DefaultMaxIterations bounds the self-driving tool loop. A model that
// keeps requesting tools — or a buggy tool whose result always re-triggers
// a call — would otherwise spin forever; the cap turns that into a
// [MaxIterationsError] instead.
const DefaultMaxIterations = 50

// Config tunes [NewMiddleware]. Every field is optional; the zero
// value yields the default loop (cap = [DefaultMaxIterations]).
//
// Tool-failure handling is NOT configurable: a tool error that isn't a
// control-flow signal (ctx cancel / a Halt (abort or HITL interrupt)) is
// always fed back to the model as a tool result so it can recover, and an
// unregistered tool is always answered with an error result rather than
// aborting the run. That recovery is the framework default; a tool author
// chooses the outcome at the source (fold the failure into the result string,
// or return an ordinary error and let the loop wrap it). See [chat.Tool.Call].
type Config struct {
	// MaxIterations caps the number of model calls the tool loop makes.
	// <= 0 falls back to [DefaultMaxIterations].
	MaxIterations int

	// FeedbackOnEmptyResponse, when true, answers an empty model reply (no
	// text and no tool calls) with a nudge and re-prompts the model once
	// instead of returning the empty reply. Default off. Unlike tool-failure
	// recovery this is a genuine behavioral choice, not error handling, so it
	// stays opt-in.
	FeedbackOnEmptyResponse bool

	// ParkStore, when non-nil, persists interrupted tool rounds so the
	// engine never sees a [FinishReasonInterrupt] chunk — the middleware
	// saves the park state on interrupt and restores it on resume, both
	// transparent to the caller. nil (the zero-value default) selects the
	// conversation-tail design instead:
	// [buildInterruptResponse] hands the interrupted round back as a
	// [FinishReasonInterrupt] response whose tail the caller re-feeds to
	// resume.
	ParkStore ParkStore

	// LoopDetection, when non-nil, enables the repeated-tool-round
	// detector: a round whose (tool, arguments, result) signature recurs
	// past the configured threshold halts the loop with a
	// [LoopDetectedError] — well before [MaxIterations]. nil (the
	// zero-value default) disables it, leaving the loop bounded only by
	// MaxIterations. See [LoopDetectionConfig].
	LoopDetection *LoopDetectionConfig

	// BeforeRound, when non-nil, is invoked before each CONTINUATION model
	// round — after a tool result is fed back, before the next model call;
	// never before the FIRST round. Any messages it returns are appended to
	// that round's request (after the tool result), so a caller can inject a
	// turn into a running loop without restarting it — e.g. "mid-run steering",
	// where a user message reaches the model on the next round. Returning nil
	// (the common case) leaves the round unchanged. The hook must not block.
	// Default nil.
	BeforeRound func(ctx context.Context) []chat.Message
}

// MaxIterationsError is returned when the tool-calling loop exceeds its
// configured iteration cap. Callers can detect it with [errors.As].
type MaxIterationsError struct {
	Limit int
}

func (e *MaxIterationsError) Error() string {
	return fmt.Sprintf("tool: loop exceeded %d iterations without a final reply", e.Limit)
}

// middleware turns the model handler into a self-driving tool-calling
// loop. When the LLM emits tool calls the middleware executes them via
// the [invoker] and re-prompts the model with the results,
// repeating until the model produces a regular reply, every tool is
// configured for direct return, or the iteration cap is hit.
//
// Use it via [NewMiddleware], which returns both call and stream
// halves so a single registration covers both paths.
//
// Example:
//
//	callMW, streamMW := toolloop.NewMiddleware()
//	resp, err := client.Chat().
//	    WithCallMiddlewares(callMW).
//	    WithStreamMiddlewares(streamMW).
//	    WithTools(myTool).
//	    Call().Response(ctx)
type middleware struct {
	maxIterations int
	feedbackEmpty bool
	parkStore     ParkStore
	loopDetection *LoopDetectionConfig
	beforeRound   func(ctx context.Context) []chat.Message
}

// NewMiddleware constructs the tool-calling middleware pair. Pass an
// optional [Config] to tune the loop; omit it for defaults.
func NewMiddleware(config ...Config) (chat.CallMiddleware, chat.StreamMiddleware) {
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	mw := &middleware{
		maxIterations: maxIterations,
		feedbackEmpty: cfg.FeedbackOnEmptyResponse,
		parkStore:     cfg.ParkStore,
		loopDetection: cfg.LoopDetection,
		beforeRound:   cfg.BeforeRound,
	}
	return mw.wrapCallHandler, mw.wrapStreamHandler
}

// loopState carries the per-loop bookkeeping threaded through the
// recursion: the 1-based model-call count and whether the one-shot
// empty-response nudge has already been spent.
type loopState struct {
	iteration    int
	emptyRetried bool
}

func (s loopState) next() loopState {
	s.iteration++
	return s
}

// nudged returns the next-iteration state with the one-shot
// empty-response retry marked as spent, so the nudge fires at most once
// per loop.
func (s loopState) nudged() loopState {
	s = s.next()
	s.emptyRetried = true
	return s
}
