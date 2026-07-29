package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

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
	switch k {
	case KindUnary:
		return "unary"
	case KindStream:
		return "stream"
	default:
		return fmt.Sprintf("MethodKind(%d)", k)
	}
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

// Replays reports whether this policy keeps a replay record at all. Unknown
// values never acquire replay semantics by accident; registry validation rejects
// them before dispatch starts.
func (p IdempotencyPolicy) Replays() bool {
	return p == IdempotencyReplayResponse || p == IdempotencyReplayRunStream
}

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
	//     ([protocol.WireValidator]), so the shape already declares it;
	//   - capability_not_negotiated — implied by CapabilityRules. A method whose
	//     gate depends on durable state rather than request shape declares it here,
	//     because no static rule can describe that trigger.
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

	// ResultNullable says the success result may be JSON null: the method answers
	// "there is none" with an absent value instead of an error.
	//
	// It cannot be derived. Nearly every handler returns a pointer, and for all but
	// one of them nil is impossible — so pointer-ness says nothing, and without this
	// the published schema would name a shape the runtime may legitimately not send,
	// leaving a client that validates its results to reject a correct frame.
	ResultNullable bool
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

// ProblemTypes returns the effective method-level failures in declaration order.
// Static capability refusals are derived from CapabilityRules; state-dependent
// refusals remain explicit in Errors because no request-shape rule can state when
// they apply.
func (m MethodMeta) ProblemTypes() []string {
	problems := slices.Clone(m.Errors)
	if len(m.CapabilityRules) > 0 &&
		!slices.Contains(problems, protocol.ErrCapabilityNotNeg.Error()) {
		problems = append(problems, protocol.ErrCapabilityNotNeg.Error())
	}
	return problems
}

// validate rejects a registration that could not be honored. A contract whose
// own metadata is inconsistent is worse than none: every generated artifact
// would inherit the inconsistency.
func (m MethodMeta) validate() error {
	if m.Name == "" {
		return errors.New("method name is required")
	}
	segments := strings.Split(m.Name, ".")
	if len(segments) < 2 || slices.Contains(segments, "") {
		return fmt.Errorf(
			"method name %q is invalid; expected dot-separated non-empty segments",
			m.Name,
		)
	}
	switch m.Kind {
	case KindUnary, KindStream:
	default:
		return fmt.Errorf(
			"%s: invalid method kind %s; expected %s or %s",
			m.Name, m.Kind, KindUnary, KindStream,
		)
	}
	switch m.Idempotency {
	case IdempotencyNone, IdempotencyReplayResponse, IdempotencyReplayRunStream:
	default:
		return fmt.Errorf(
			"%s: invalid idempotency policy %s; expected %s, %s or %s",
			m.Name,
			m.Idempotency,
			IdempotencyNone,
			IdempotencyReplayResponse,
			IdempotencyReplayRunStream,
		)
	}
	if !m.Stability.Valid() {
		return fmt.Errorf(
			"%s: invalid stability %q; expected %q or %q",
			m.Name,
			m.Stability,
			protocol.StabilityStable,
			protocol.StabilityExperimental,
		)
	}
	for index, problem := range m.Errors {
		if !IsMethodProblemType(problem) {
			return fmt.Errorf("%s: %q is not a declarable problem type", m.Name, problem)
		}
		if slices.Contains(m.Errors[:index], problem) {
			return fmt.Errorf("%s: problem type %q is declared twice", m.Name, problem)
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
	if m.ResultNullable && (m.Result == nil || m.Result.Kind() != reflect.Pointer) {
		return fmt.Errorf("%s: a nullable result needs a pointer to be nil", m.Name)
	}
	for ruleIndex, rule := range m.CapabilityRules {
		if len(rule.Requires) == 0 {
			return fmt.Errorf(
				"%s: capability rule %d must require at least one feature",
				m.Name, ruleIndex,
			)
		}
		for conditionIndex, condition := range rule.When {
			if slices.Contains(rule.When[:conditionIndex], condition) {
				return fmt.Errorf(
					"%s: capability rule %d repeats condition for field %q with operator %s",
					m.Name, ruleIndex, condition.Field, condition.Operator,
				)
			}
			if err := validateFieldCondition(m.Name, m.Params, condition); err != nil {
				return err
			}
		}
		for featureIndex, feature := range rule.Requires {
			if _, published := protocol.LookupFeature(feature); !published {
				return fmt.Errorf(
					"%s: capability rule %d requires unknown feature %q",
					m.Name, ruleIndex, feature,
				)
			}
			if slices.Contains(rule.Requires[:featureIndex], feature) {
				return fmt.Errorf(
					"%s: capability rule %d requires feature %q twice",
					m.Name, ruleIndex, feature,
				)
			}
		}
	}
	if len(m.CapabilityRules) > 0 &&
		slices.Contains(m.Errors, protocol.ErrCapabilityNotNeg.Error()) {
		return fmt.Errorf(
			"%s: capability_not_negotiated is derived from capability rules and must not be declared twice",
			m.Name,
		)
	}
	return nil
}

func (p IdempotencyPolicy) String() string {
	switch p {
	case IdempotencyNone:
		return "none"
	case IdempotencyReplayResponse:
		return "replayResponse"
	case IdempotencyReplayRunStream:
		return "replayRunStream"
	default:
		return fmt.Sprintf("IdempotencyPolicy(%d)", p)
	}
}

func (o ConditionOperator) String() string {
	switch o {
	case OperatorPresent:
		return "present"
	case OperatorEquals:
		return "equals"
	default:
		return fmt.Sprintf("ConditionOperator(%d)", o)
	}
}

func validateFieldCondition(owner string, shape reflect.Type, condition FieldCondition) error {
	if condition.Field == "" {
		return fmt.Errorf("%s: condition must name a field", owner)
	}
	if err := protocol.HasWirePath(shape, condition.Field); err != nil {
		return fmt.Errorf("%s condition field %q: %w", owner, condition.Field, err)
	}
	switch condition.Operator {
	case OperatorPresent:
		if condition.Value != "" {
			return fmt.Errorf(
				"%s condition field %q: operator %s does not accept value %q",
				owner, condition.Field, condition.Operator, condition.Value,
			)
		}
	case OperatorEquals:
		if condition.Value == "" {
			return fmt.Errorf(
				"%s condition field %q: operator %s requires a non-empty value",
				owner, condition.Field, condition.Operator,
			)
		}
	default:
		return fmt.Errorf(
			"%s condition field %q: invalid operator %s; expected %s or %s",
			owner,
			condition.Field,
			condition.Operator,
			OperatorPresent,
			OperatorEquals,
		)
	}
	return nil
}
