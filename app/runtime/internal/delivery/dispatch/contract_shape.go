package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// This file holds the shape half of the contract: which wire types are closed
// unions, which cross-field constraints they carry, and which shared-state keys
// exist. The method half is contract.go.
//
// Go models these unions as FLAT tag-discriminated structs (one `type` field plus
// the optional fields that tag allows), which is the right wire shape but tells a
// reader nothing about which fields go with which tag. Reflection cannot recover
// that — a discriminator is not a Go concept — so it is declared here and CHECKED
// against the struct. The check is the point: a spec that names a field the struct
// does not have, or a struct field no variant accounts for, fails at startup
// instead of silently producing a schema that permits an illegal frame.

// UnionSpec declares one closed tag-discriminated union (contract §11.2).
type UnionSpec struct {
	GoType reflect.Type
	// Discriminator is the JSON field carrying the tag. API.md §2.1 fixes it at
	// `type` for every first-party union, with no exceptions.
	Discriminator string
	Variants      []VariantSpec
}

// VariantSpec is one tag of a union and the fields that tag brings. Names are
// JSON field names, dotted for a nested frame (`payload.tool`).
type VariantSpec struct {
	Tag      string
	Required []string
	Optional []string
}

// ObjectConstraintSpec declares cross-field rules inside ONE DTO (contract
// §11.2). It is deliberately frame-local: an invariant spanning runs, interrupts
// or the store is a transaction concern and is declared in the application ring,
// so nothing here ever needs a repository to decide.
type ObjectConstraintSpec struct {
	GoType reflect.Type
	Rules  []PresenceRule
}

// PresenceRule says which fields must (or must not) be there when a condition
// holds. Required and Forbidden are JSON field names, dotted for a nested frame.
type PresenceRule struct {
	When      []FieldCondition
	Required  []string
	Forbidden []string
}

// StateSnapshotScope is how far one shared-state key reaches.
type StateSnapshotScope string

// StateScopeSession is the only scope today: a state key belongs to its session
// and every Run in that session reads and writes the same value.
const StateScopeSession StateSnapshotScope = "session"

// StateSnapshotWriter is who is allowed to publish a key's value.
type StateSnapshotWriter string

// StateWriterRootRun is the only writer today: the session's root Run. A child
// Run cannot publish shared state (there are none — features.subagents is off),
// which is why the rule has nothing to violate yet.
const StateWriterRootRun StateSnapshotWriter = "rootRun"

// StateKeySpec declares one first-party `state.snapshot` key (contract §11.2).
// RecoveryMethod names how a client that missed the event gets the current value
// — a key with no recovery path would break reconnect, so it is stated, not
// assumed.
type StateKeySpec struct {
	Key            string
	RecoveryMethod string
	Scope          StateSnapshotScope
	Writer         StateSnapshotWriter
	Feature        string
	Stability      protocol.Stability

	// PayloadType is the Go type of the value published under this key.
	//
	// The state envelope is a `map[string]any` — deliberately, so a new key needs no
	// wire change — which means the shape of a key's value is invisible to
	// reflection. Declaring it is the only way the published contract can say what
	// `state.snapshot` actually carries; without it a client reads "some JSON".
	PayloadType reflect.Type
}

// CarriedSpec declares a wire type the method graph cannot reach.
//
// The artifact walk starts from the registered methods, so a shape that rides
// somewhere else is invisible to it: `params._meta` is stripped before typed
// decoding, and a tool result is deliberately opaque JSON. Both are on the wire and
// a client has to construct or render them, so a contract that omits their shape is
// incomplete.
//
// Carrier says WHERE it rides, in wire terms. A bare list of types would publish
// the shapes without answering the only question a reader has about them.
type CarriedSpec struct {
	Carrier string
	GoType  reflect.Type
}

// ConstraintKind is a value constraint a field's JSON type does not express.
type ConstraintKind uint8

const (
	// ConstraintNonEmpty rejects the empty string. A required id whose value is ""
	// names nothing, and every transport and generated client should refuse it in
	// the same place rather than each handler deciding.
	ConstraintNonEmpty ConstraintKind = iota
	// ConstraintPositive rejects zero. A revision or count of zero is not a value
	// the caller could have meant.
	ConstraintPositive
)

func (k ConstraintKind) String() string {
	if k == ConstraintPositive {
		return "positive"
	}
	return "nonEmpty"
}

// FieldConstraint is one field's value constraint. Field is a dotted JSON path.
type FieldConstraint struct {
	Field string
	Kind  ConstraintKind
}

// FieldConstraintSpec declares the value constraints of one request shape.
//
// These are the checks reflection cannot see: that a string must be non-empty,
// that a number must exceed zero. Closed-enum membership is NOT declared here —
// the enum's value set is already declared, so the check is derived from it, and
// declaring it twice would let the two disagree.
//
// The declaration is the single source: the Go validator, the TypeScript validator
// and the schema's minLength / minimum are all generated from it, which is what
// makes the three equivalent by construction instead of by a reminder.
type FieldConstraintSpec struct {
	GoType      reflect.Type
	Constraints []FieldConstraint
}

// Shapes is the registered shape contract. It is separate from the method
// registry because a union is not a method: several methods carry the same union,
// and the artifacts generated from it (oneOf + discriminator, if/then) are
// per-type, not per-method.
type Shapes struct {
	unions      []UnionSpec
	constraints []ObjectConstraintSpec
	stateKeys   []StateKeySpec
	carried     []CarriedSpec
	values      []FieldConstraintSpec
}

func (s *Shapes) Unions() []UnionSpec { return s.unions }

// Constraints returns every registered rule PLUS the rules a shape inherits by
// embedding another constrained shape.
//
// encoding/json inlines an embedded struct's fields, so a rule about those fields
// is true of the embedding type too — but nothing said so, and a rule registered
// for RunSummary silently stopped applying to the RunRef that carries it. The
// inheritance is READ OFF the Go embedding rather than declared, because the
// embedding is already the declaration: a second statement of "RunRef composes
// RunSummary" could disagree with the struct.
func (s *Shapes) Constraints() []ObjectConstraintSpec {
	byType := make(map[reflect.Type]ObjectConstraintSpec, len(s.constraints))
	for _, spec := range s.constraints {
		byType[spec.GoType] = spec
	}
	out := make([]ObjectConstraintSpec, 0, len(s.constraints))
	for _, spec := range s.constraints {
		for _, embedded := range protocol.WireEmbeds(spec.GoType) {
			inherited, ok := byType[embedded]
			if !ok {
				continue
			}
			spec.Rules = append(slices.Clone(inherited.Rules), spec.Rules...)
		}
		out = append(out, spec)
	}
	return out
}
func (s *Shapes) StateKeys() []StateKeySpec               { return s.stateKeys }
func (s *Shapes) Carried() []CarriedSpec                  { return s.carried }
func (s *Shapes) ValueConstraints() []FieldConstraintSpec { return s.values }

func (s *Shapes) union(spec UnionSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid union spec: " + err.Error())
	}
	s.unions = append(s.unions, spec)
}

func (s *Shapes) constraint(spec ObjectConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid object constraint spec: " + err.Error())
	}
	s.constraints = append(s.constraints, spec)
}

func (s *Shapes) stateKey(spec StateKeySpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid state key spec: " + err.Error())
	}
	s.stateKeys = append(s.stateKeys, spec)
}

func (s *Shapes) valueConstraint(spec FieldConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid value constraint spec: " + err.Error())
	}
	s.values = append(s.values, spec)
}

func (s *Shapes) carriedShape(spec CarriedSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid carried shape spec: " + err.Error())
	}
	s.carried = append(s.carried, spec)
}

// validate checks a union spec against the struct it describes.
func (u UnionSpec) validate() error {
	if u.GoType == nil || u.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("union spec needs a struct type, got %v", u.GoType)
	}
	name := u.GoType.Name()
	if u.Discriminator != "type" {
		return fmt.Errorf("%s: discriminator is %q — API.md §2.1 fixes it at \"type\"", name, u.Discriminator)
	}
	if err := protocol.HasWirePath(u.GoType, u.Discriminator); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(u.Variants) == 0 {
		return fmt.Errorf("%s: a closed union with no variants describes nothing", name)
	}
	accounted := []string{u.Discriminator}
	tags := make(map[string]bool, len(u.Variants))
	for _, variant := range u.Variants {
		if variant.Tag == "" {
			return fmt.Errorf("%s: a variant needs a tag", name)
		}
		if tags[variant.Tag] {
			return fmt.Errorf("%s: variant %q is declared twice", name, variant.Tag)
		}
		tags[variant.Tag] = true
		for _, field := range slices.Concat(variant.Required, variant.Optional) {
			if err := protocol.HasWirePath(u.GoType, field); err != nil {
				return fmt.Errorf("%s variant %q: %w", name, variant.Tag, err)
			}
			// A nested declaration accounts for the frame that holds it: claiming
			// `payload.tool` claims `payload`.
			root := strings.Split(field, ".")[0]
			if !slices.Contains(accounted, root) {
				accounted = append(accounted, root)
			}
		}
	}
	// The drift that actually happens: a field is added to the struct and no
	// variant claims it, so the generated schema would allow it under every tag.
	for _, field := range protocol.WireFieldNames(u.GoType) {
		if !slices.Contains(accounted, field) {
			return fmt.Errorf("%s: field %q belongs to no variant — every field of a closed union must name its tag", name, field)
		}
	}
	return nil
}

func (o ObjectConstraintSpec) validate() error {
	if o.GoType == nil || o.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("object constraint spec needs a struct type, got %v", o.GoType)
	}
	name := o.GoType.Name()
	if len(o.Rules) == 0 {
		return fmt.Errorf("%s: a constraint spec with no rules constrains nothing", name)
	}
	for index, rule := range o.Rules {
		if len(rule.Required) == 0 && len(rule.Forbidden) == 0 {
			return fmt.Errorf("%s rule %d: states neither a required nor a forbidden field", name, index)
		}
		for _, condition := range rule.When {
			if err := protocol.HasWirePath(o.GoType, condition.Field); err != nil {
				return fmt.Errorf("%s rule %d condition: %w", name, index, err)
			}
		}
		for _, field := range slices.Concat(rule.Required, rule.Forbidden) {
			if err := protocol.HasWirePath(o.GoType, field); err != nil {
				return fmt.Errorf("%s rule %d: %w", name, index, err)
			}
		}
	}
	return nil
}

func (f FieldConstraintSpec) validate() error {
	if f.GoType == nil || f.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("value constraint spec needs a struct type, got %v", f.GoType)
	}
	name := f.GoType.Name()
	if len(f.Constraints) == 0 {
		return fmt.Errorf("%s: a constraint spec with no constraints constrains nothing", name)
	}
	for _, constraint := range f.Constraints {
		selector, leaf, ok := protocol.GoPath(f.GoType, constraint.Field)
		if !ok {
			return fmt.Errorf("%s: no JSON field %q", name, constraint.Field)
		}
		kind := protocol.Deref(leaf.Type).Kind()
		switch constraint.Kind {
		case ConstraintNonEmpty:
			if kind != reflect.String {
				return fmt.Errorf("%s.%s is %s; only a string can be non-empty", name, selector, kind)
			}
		case ConstraintPositive:
			if kind != reflect.Uint64 && kind != reflect.Int && kind != reflect.Int64 {
				return fmt.Errorf("%s.%s is %s; only a number can be positive", name, selector, kind)
			}
		}
	}
	return nil
}

func (c CarriedSpec) validate() error {
	switch {
	case c.Carrier == "":
		return errors.New("carried shape spec needs the wire member it rides in")
	case c.GoType == nil:
		return fmt.Errorf("carrier %q has no type", c.Carrier)
	}
	return nil
}

func (k StateKeySpec) validate() error {
	switch {
	case k.Key == "":
		return fmt.Errorf("state key spec needs a key")
	case k.RecoveryMethod == "":
		return fmt.Errorf("state key %q: a key with no recovery method breaks reconnect", k.Key)
	case k.Scope == "":
		return fmt.Errorf("state key %q: scope is required", k.Key)
	case k.Writer == "":
		return fmt.Errorf("state key %q: writer is required", k.Key)
	case k.Feature == "":
		return fmt.Errorf("state key %q: feature gate is required", k.Key)
	case k.Stability == "":
		return fmt.Errorf("state key %q: stability is required", k.Key)
	case k.PayloadType == nil:
		return fmt.Errorf("state key %q: payload type is required — an untyped key publishes \"some JSON\"", k.Key)
	}
	if _, ok := contract.Lookup(k.RecoveryMethod); !ok {
		return fmt.Errorf("state key %q: recovery method %q is not a registered method", k.Key, k.RecoveryMethod)
	}
	return nil
}
