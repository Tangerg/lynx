package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// MethodKind separates the two response shapes a method can have: one JSON-RPC
// result, or a result followed by this call's own event stream (TRANSPORT §6.4).
// It is set by the registration factory, never declared by hand — the factory is
// the only thing that knows which pipeline it built.
type MethodKind uint8

const (
	KindUnary MethodKind = iota
	KindStream
)

func (k MethodKind) String() string {
	if k == KindStream {
		return "stream"
	}
	return "unary"
}

// IdempotencyPolicy says what an `Idempotency-Key` retry of this method must do
// (TRANSPORT §6.2 / §10). It replaces a hand-kept list of replay-protected
// method names: a method declares its own retry semantics where it is
// registered, so adding a mutation cannot silently skip replay protection.
type IdempotencyPolicy uint8

const (
	// IdempotencyNone keeps no replay record. Reads, and mutations whose repeat
	// is indistinguishable from the first call, need none.
	IdempotencyNone IdempotencyPolicy = iota

	// IdempotencyReplayResponse caches the first response and returns it verbatim
	// to a same-key retry, so the business handler runs exactly once.
	IdempotencyReplayResponse

	// IdempotencyReplayRunStream caches the response AND re-attaches the retry to
	// the run it already opened, instead of starting a second one. Only a method
	// that opens a run needs this: replaying the cached ack alone would hand the
	// client a runId with no stream behind it.
	IdempotencyReplayRunStream
)

// Replays reports whether this policy keeps a replay record at all.
func (p IdempotencyPolicy) Replays() bool { return p != IdempotencyNone }

// ConditionOperator is how a [FieldCondition] tests one request field.
type ConditionOperator uint8

const (
	// OperatorPresent matches when the field is present and not its zero value —
	// an absent optional field and an explicit empty one are the same request.
	OperatorPresent ConditionOperator = iota
	// OperatorEquals matches when the field's JSON value equals Value.
	OperatorEquals
)

// FieldCondition tests one field of the request frame. Field is a dotted path
// into the same frame; it never reaches outside the request (contract §11.2), so
// evaluating a condition can never require a store lookup.
type FieldCondition struct {
	Field    string
	Operator ConditionOperator
	Value    string
}

// CapabilityRule states which server features a call needs.
//
// An empty When means the whole method needs them. A non-empty When means only
// the requests matching it do — which is how a method stays available in its
// default form while one of its options is gated (contract §11.1).
//
// The rule is the ONLY statement of the requirement: discovery advertises the
// same feature map the gate reads, so an advertised feature is callable and a
// disabled one is refused before any use case runs. A handler that re-checks
// `if !s.features.X` is a second author of the same rule, and the two drift.
type CapabilityRule struct {
	When     []FieldCondition
	Requires []string
}

// MethodMeta is everything about a method that is not its handler: what it is
// called, which retry semantics it has, which errors it may return, and which
// features it needs. Registration is the single place this is stated.
type MethodMeta struct {
	// Name is the JSON-RPC method name (API.md §2.5), dotted <domain>.<verb>.
	Name string

	// Kind is filled in by the registration factory.
	Kind MethodKind

	Idempotency IdempotencyPolicy

	// Errors is the method-SPECIFIC ProblemData.type values its contract documents
	// (API.md §7 per-method "错误" lines). It deliberately does not include:
	//
	//   - internal_error — the universal fallback on every method;
	//   - invalid_params — carried by any request whose type states constraints
	//     ([protocol.Validator]), so the shape already declares it;
	//   - capability_not_negotiated — implied by CapabilityRules, and stating it
	//     twice would let the two disagree.
	//
	// It states WHICH errors are possible, never WHEN one fires — the trigger is
	// a use-case decision and does not belong in wire metadata (contract §11.1).
	//
	// An absent entry means "the contract has not documented it", not "it cannot
	// happen": this is the audited set, and closing a gap is a contract edit here.
	Errors []string

	CapabilityRules []CapabilityRule

	Stability protocol.Stability

	// Params, Result and Event are the Go types of the method's wire frames, filled
	// in by the registration factory from its own type parameters — the one place
	// that knows them without a second declaration to keep in step.
	//
	// They exist for the schema walker: an artifact generator needs the type graph,
	// not just the names. Result is nil for an ack-only method (its success carries
	// no data) and Event is nil for a unary one.
	Params reflect.Type
	Result reflect.Type
	Event  reflect.Type
}

// Features returns every feature key this method's rules can require, in
// declaration order and without duplicates.
func (m MethodMeta) Features() []string {
	var out []string
	for _, rule := range m.CapabilityRules {
		for _, feature := range rule.Requires {
			if !slices.Contains(out, feature) {
				out = append(out, feature)
			}
		}
	}
	return out
}

// knownProblemTypes is the closed first-party ProblemData.type vocabulary a
// method may declare. It exists so a typo in a registration fails at startup
// rather than shipping an error name no client has copy for.
var knownProblemTypes = []string{
	protocol.ErrSessionNotFound.Error(),
	protocol.ErrRunNotFound.Error(),
	protocol.ErrItemNotFound.Error(),
	protocol.ErrRunAlreadyDone.Error(),
	protocol.ErrInterruptNotOpen.Error(),
	protocol.ErrSessionBusy.Error(),
	protocol.ErrRevisionConflict.Error(),
	protocol.ErrCwdUnavailable.Error(),
	protocol.ErrPathOutsideRoot.Error(),
	protocol.ErrVcsUnavailable.Error(),
	protocol.ErrCheckpointUnavailable.Error(),
	protocol.ErrUnsupportedMime.Error(),
	protocol.ErrProviderError.Error(),
}

// validate rejects a registration that could not be honored. A contract whose
// own metadata is inconsistent is worse than none: every generated artifact
// would inherit the inconsistency.
func (m MethodMeta) validate() error {
	if m.Name == "" {
		return errors.New("method name is required")
	}
	if m.Stability == "" {
		return fmt.Errorf("%s: stability is required", m.Name)
	}
	for _, problem := range m.Errors {
		if !slices.Contains(knownProblemTypes, problem) {
			return fmt.Errorf("%s: %q is not a declarable problem type", m.Name, problem)
		}
	}
	if m.Kind == KindStream && m.Idempotency == IdempotencyReplayResponse {
		return fmt.Errorf("%s: a streaming method replays by re-attaching, not by returning a cached ack", m.Name)
	}
	if m.Kind == KindUnary && m.Idempotency == IdempotencyReplayRunStream {
		return fmt.Errorf("%s: only a run-opening stream can replay by re-attaching", m.Name)
	}
	if m.Params == nil {
		return fmt.Errorf("%s: params type is required — a method with no schema cannot be published", m.Name)
	}
	if (m.Kind == KindStream) != (m.Event != nil) {
		return fmt.Errorf("%s: a stream declares its event type and only a stream has one", m.Name)
	}
	for _, rule := range m.CapabilityRules {
		if len(rule.Requires) == 0 {
			return fmt.Errorf("%s: a capability rule must require at least one feature", m.Name)
		}
		for _, condition := range rule.When {
			if condition.Field == "" {
				return fmt.Errorf("%s: a capability condition must name a field", m.Name)
			}
			if condition.Operator == OperatorEquals && condition.Value == "" {
				return fmt.Errorf("%s: an equals condition on %q needs a value", m.Name, condition.Field)
			}
		}
	}
	return nil
}

func (p IdempotencyPolicy) String() string {
	switch p {
	case IdempotencyReplayResponse:
		return "replayResponse"
	case IdempotencyReplayRunStream:
		return "replayRunStream"
	default:
		return "none"
	}
}

func (o ConditionOperator) String() string {
	if o == OperatorEquals {
		return "equals"
	}
	return "present"
}
