package turn

import (
	"errors"
	"fmt"
	"slices"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// ErrInputRequired reports that a turn has neither text nor media to send.
var ErrInputRequired = errors.New("turn: input required")

// ErrIncompleteModelSelection reports a provider/model pair where only one side
// was supplied. Turn model selection is explicit: both are set, or both empty.
var ErrIncompleteModelSelection = errors.New("turn: incomplete model selection")

// ErrInvalidTurnLimit reports a negative turn budget / step cap. Limits use
// zero as "unlimited", so negative values have no domain meaning.
var ErrInvalidTurnLimit = errors.New("turn: invalid limit")

// ErrInvalidTurnOptions reports malformed per-run generation tuning.
var ErrInvalidTurnOptions = errors.New("turn: invalid options")

// StartTurnRequest is the input to [Dispatcher.StartTurn]. SessionID
// binds the turn to its conversation; Message is the user's input.
type StartTurnRequest struct {
	SessionID string
	Message   string

	// Media carries the turn's image attachments (runs.start input image
	// blocks). Nil for a text-only turn. They ride the user message to the
	// model as UserMessage.Media; only models whose catalog modalities accept
	// image input should be sent them (gated before StartTurn).
	Media []*media.Media

	// Cwd is the session's working directory — the project root the turn's
	// filesystem + shell tools run in. Resolved from Session.cwd by the
	// caller (runs.start). Empty falls back to the engine's default workdir.
	Cwd string

	// Provider + Model select the model this turn runs against (the wire
	// runs.start{providerId, model}). Both empty uses the runtime's default;
	// both set resolves that provider+model client via the clientResolver and
	// runs the turn against it. The provider is explicit — never inferred.
	Provider string
	Model    string

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. On
	// overrun the turn stops cleanly after the current round and ends
	// with Reason=[execution.OutcomeMaxBudget], the partial reply already
	// streamed. In-process / automated callers set this; it is not
	// (yet) carried on the wire.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost the same way MaxBudget caps
	// tokens (0 = no cap). Needs a configured pricing hook; same
	// execution.OutcomeMaxBudget stop. Also not (yet) on the wire.
	MaxCostUSD float64

	// MaxSteps caps cumulative model calls across the root and child delegation
	// tree; 0 = unlimited. On overrun the turn stops cleanly before the next
	// model call with Reason=[execution.OutcomeMaxSteps] (distinct from the
	// token/cost budget).
	MaxSteps int

	// Options carries per-run generation tuning. The turn keeps model
	// selection explicit on Provider/Model; Options.Model is therefore invalid
	// here and must stay empty.
	Options *corechat.Options

	// InterruptKinds are the HITL kinds the client starting this turn can
	// answer. Nil or empty means the turn must not surface any HITL interrupt;
	// it auto-denies instead of parking on an unanswerable prompt.
	InterruptKinds []string
}

// snapshot returns the immutable protocol values the asynchronous turn owns.
// Runtime collaborators such as clients, observers, and callbacks keep their
// documented shared concurrency semantics and are attached later.
func (r StartTurnRequest) snapshot() StartTurnRequest {
	snapshot := r
	if r.Options != nil {
		options := r.Options.Clone()
		snapshot.Options = &options
	}
	if r.Media != nil {
		snapshot.Media = make([]*media.Media, len(r.Media))
		for index := range r.Media {
			snapshot.Media[index] = r.Media[index].Clone()
		}
	}
	snapshot.InterruptKinds = slices.Clone(r.InterruptKinds)
	return snapshot
}

// Validate rejects malformed turn drafts before they bind to a session or
// launch an agent process.
func (r StartTurnRequest) Validate() error {
	if r.Message == "" && len(r.Media) == 0 {
		return ErrInputRequired
	}
	if (r.Model == "") != (r.Provider == "") {
		return ErrIncompleteModelSelection
	}
	if r.MaxBudget < 0 {
		return fmt.Errorf("%w: MaxBudget must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxCostUSD < 0 {
		return fmt.Errorf("%w: MaxCostUSD must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxSteps < 0 {
		return fmt.Errorf("%w: MaxSteps must be non-negative", ErrInvalidTurnLimit)
	}
	if err := validateOptions(r.Options); err != nil {
		return err
	}
	return nil
}

func validateOptions(options *corechat.Options) error {
	if options == nil {
		return nil
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTurnOptions, err)
	}
	if options.Model != "" {
		return fmt.Errorf("%w: Options.Model must stay empty; use Provider and Model", ErrInvalidTurnOptions)
	}
	return nil
}

// TurnHandle uniquely identifies an in-flight turn. Returned by
// [Dispatcher.StartTurn] and used to address subsequent operations
// (steering injection, cancellation).
type TurnHandle struct {
	SessionID string
	TurnID    string

	// state keeps an unclaimed process-creation failure stream reachable after
	// the failed turn leaves the live registry. Reconstructed control handles
	// still resolve exclusively through TurnID.
	state *turnState
}

// RehydrateRequest carries the inputs to rebuild a parked turn from a persisted
// process snapshot after a restart. ProcessID is the agent-process snapshot key
// recorded on the open interrupt; TurnID reuses its durable executor handle so
// a failed pre-opening attempt remains discoverable on retry; SessionID rebinds
// chat history. The decision is delivered later through Resume.
type RehydrateRequest struct {
	SessionID string
	TurnID    string
	ProcessID string

	// Provider + Model are the parked run's per-run model selection, persisted
	// on the interrupt. Both set re-resolves that client so the continuation
	// runs against the SAME model it parked on; both empty (or no resolver)
	// runs on the engine default. The provider is explicit — never inferred.
	Provider string
	Model    string
}
