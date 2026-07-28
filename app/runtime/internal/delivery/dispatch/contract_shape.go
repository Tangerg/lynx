package dispatch

import (
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
}

// Shapes is the registered shape contract. It is separate from the method
// registry because a union is not a method: several methods carry the same union,
// and the artifacts generated from it (oneOf + discriminator, if/then) are
// per-type, not per-method.
type Shapes struct {
	unions      []UnionSpec
	constraints []ObjectConstraintSpec
	stateKeys   []StateKeySpec
}

func (s *Shapes) Unions() []UnionSpec                 { return s.unions }
func (s *Shapes) Constraints() []ObjectConstraintSpec { return s.constraints }
func (s *Shapes) StateKeys() []StateKeySpec           { return s.stateKeys }

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

// validate checks a union spec against the struct it describes.
func (u UnionSpec) validate() error {
	if u.GoType == nil || u.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("union spec needs a struct type, got %v", u.GoType)
	}
	name := u.GoType.Name()
	if u.Discriminator != "type" {
		return fmt.Errorf("%s: discriminator is %q — API.md §2.1 fixes it at \"type\"", name, u.Discriminator)
	}
	if err := hasJSONField(u.GoType, u.Discriminator); err != nil {
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
			if err := hasJSONField(u.GoType, field); err != nil {
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
	for _, field := range jsonFields(u.GoType) {
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
			if err := hasJSONField(o.GoType, condition.Field); err != nil {
				return fmt.Errorf("%s rule %d condition: %w", name, index, err)
			}
		}
		for _, field := range slices.Concat(rule.Required, rule.Forbidden) {
			if err := hasJSONField(o.GoType, field); err != nil {
				return fmt.Errorf("%s rule %d: %w", name, index, err)
			}
		}
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
	}
	if _, ok := contract.Lookup(k.RecoveryMethod); !ok {
		return fmt.Errorf("state key %q: recovery method %q is not a registered method", k.Key, k.RecoveryMethod)
	}
	return nil
}

// hasJSONField reports whether a dotted JSON path addresses a real field.
func hasJSONField(root reflect.Type, path string) error {
	current := root
	for _, segment := range strings.Split(path, ".") {
		field, ok := lookupJSONField(current, segment)
		if !ok {
			return fmt.Errorf("no JSON field %q on %s", segment, current.Name())
		}
		current = deref(field)
	}
	return nil
}

func lookupJSONField(owner reflect.Type, jsonName string) (reflect.Type, bool) {
	if owner.Kind() != reflect.Struct {
		return nil, false
	}
	for index := range owner.NumField() {
		field := owner.Field(index)
		name, embedded := jsonNameOf(field)
		if embedded {
			// An embedded struct inlines its fields onto the same JSON object, so a
			// path segment may address one of them.
			if inner, ok := lookupJSONField(deref(field.Type), jsonName); ok {
				return inner, true
			}
			continue
		}
		if name == jsonName {
			return field.Type, true
		}
	}
	return nil, false
}

// jsonFields lists every JSON field name a struct marshals, following embedded
// structs the way encoding/json inlines them.
func jsonFields(owner reflect.Type) []string {
	var out []string
	for index := range owner.NumField() {
		field := owner.Field(index)
		name, embedded := jsonNameOf(field)
		if embedded {
			out = append(out, jsonFields(deref(field.Type))...)
			continue
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// jsonNameOf returns a field's wire name, or embedded=true when the field is an
// anonymous struct whose fields are inlined. A "-" tag or an unexported field
// yields "" — it is not on the wire, so no variant needs to claim it.
func jsonNameOf(field reflect.StructField) (name string, embedded bool) {
	tag, tagged := field.Tag.Lookup("json")
	if field.Anonymous && (!tagged || tag == "") {
		return "", true
	}
	if !field.IsExported() {
		return "", false
	}
	if !tagged {
		return field.Name, false
	}
	name = strings.Split(tag, ",")[0]
	if name == "-" {
		return "", false
	}
	if name == "" {
		return field.Name, false
	}
	return name, false
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}
