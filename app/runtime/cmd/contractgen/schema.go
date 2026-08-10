package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/contractcatalog"
	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
)

// The schema walker turns the registered wire types into JSON Schema.
//
// Reflection alone would produce a schema that says `type: object` for a closed
// union and `type: string` for a three-word enum — technically true, useless as a
// contract, and laxer than the runtime. So the walk reads the SPECS for everything
// reflection cannot see (discriminators, variant field lists, presence rules,
// enum value sets) and reads the Go types for everything it can (field names,
// optionality, nesting). Neither half guesses at the other's job.
//
// What it deliberately does NOT emit on SHARED DEFINITIONS is
// `additionalProperties: false`. A result may grow optional fields and an older
// client must keep accepting it. Requests are different: dispatch strictly
// decodes their DTOs, so OpenRPC closes each contextual x-lyra-requestFrame with
// `unevaluatedProperties: false` while leaving the reusable result shapes open.
//
// Nor does it widen a required field to accept null. A nil Go slice or map DOES
// marshal to null, so a mechanical reading would publish `T[] | null` on most list
// fields — but `required` already states the contract: the field is there, with a
// value of that type. Publishing the null would push a runtime defect onto every
// client as defensive code, forever. The validator rejects it instead, which is
// where a broken frame should surface.

// refPrefix is where a definition lives inside the bundle. Other artifacts point
// at the same bundle rather than carrying a second copy of the shapes.
const refPrefix = "#/$defs/"

type schemaType string

const (
	schemaTypeArray   schemaType = "array"
	schemaTypeBoolean schemaType = "boolean"
	schemaTypeInteger schemaType = "integer"
	schemaTypeNull    schemaType = "null"
	schemaTypeNumber  schemaType = "number"
	schemaTypeObject  schemaType = "object"
	schemaTypeString  schemaType = "string"
)

// schema is a JSON Schema node, holding only the keywords this contract needs.
//
// Properties values are `any` because a forbidden field is the boolean schema
// `false` — the one keyword that says "this may not be present at all", which is
// how a union variant excludes another variant's fields.
type schema struct {
	Ref              string         `json:"$ref,omitempty"`
	Type             schemaType     `json:"type,omitempty"`
	Format           string         `json:"format,omitempty"`
	ContentEncoding  string         `json:"contentEncoding,omitempty"`
	Enum             []string       `json:"enum,omitempty"`
	MinLength        *int           `json:"minLength,omitempty"`
	MaxLength        *int           `json:"maxLength,omitempty"`
	Const            string         `json:"const,omitempty"`
	Pattern          string         `json:"pattern,omitempty"`
	TypeScriptType   string         `json:"-"`
	Minimum          *int64         `json:"minimum,omitempty"`
	Maximum          *int64         `json:"maximum,omitempty"`
	MinItems         *int           `json:"minItems,omitempty"`
	MinProperties    *int           `json:"minProperties,omitempty"`
	UniqueItems      bool           `json:"uniqueItems,omitempty"`
	Items            *schema        `json:"items,omitempty"`
	Properties       map[string]any `json:"properties,omitempty"`
	AdditionalProps  any            `json:"additionalProperties,omitempty"`
	UnevaluatedProps *bool          `json:"unevaluatedProperties,omitempty"`
	Required         []string       `json:"required,omitempty"`
	OneOf            []*schema      `json:"oneOf,omitempty"`
	AnyOf            []*schema      `json:"anyOf,omitempty"`
	AllOf            []*schema      `json:"allOf,omitempty"`
	If               *schema        `json:"if,omitempty"`
	Then             *schema        `json:"then,omitempty"`
}

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
)

// schemaSet is the walked type graph: one definition per named wire type, plus
// the specs that tell the walk what reflection cannot see.
type schemaSet struct {
	defs   map[string]*schema
	origin map[string]reflect.Type
	enums  map[reflect.Type][]string

	unions      map[reflect.Type]dispatch.UnionSpec
	constraints map[reflect.Type][]dispatch.PresenceRule
	values      map[reflect.Type][]dispatch.FieldConstraint
}

func newSchemaSet(shapes *dispatch.Shapes) *schemaSet {
	set := &schemaSet{
		defs:        make(map[string]*schema),
		origin:      make(map[string]reflect.Type),
		enums:       make(map[reflect.Type][]string),
		unions:      make(map[reflect.Type]dispatch.UnionSpec),
		constraints: make(map[reflect.Type][]dispatch.PresenceRule),
		values:      make(map[reflect.Type][]dispatch.FieldConstraint),
	}
	for _, union := range shapes.Unions() {
		set.unions[union.GoType] = union
	}
	for _, constraint := range shapes.Constraints() {
		set.constraints[constraint.GoType] = append(set.constraints[constraint.GoType], constraint.Rules...)
	}
	for _, spec := range shapes.ValueConstraints() {
		set.values[spec.GoType] = append(set.values[spec.GoType], spec.Constraints...)
	}
	return set
}

// Definitions returns the walked definitions keyed by name. Names are sorted by
// the JSON encoder, so the bundle's diff is stable across runs.
func (s *schemaSet) Definitions() map[string]*schema { return s.defs }

func (s *schemaSet) registerEnum(t reflect.Type, values []string) {
	if t.Kind() != reflect.String || t.Name() == "" || len(values) == 0 {
		panic(fmt.Sprintf("contractgen: invalid external enum %s with values %v", t, values))
	}
	if existing, ok := s.enums[t]; ok && !slices.Equal(existing, values) {
		panic(fmt.Sprintf("contractgen: external enum %s has conflicting values %v and %v", t, existing, values))
	}
	s.enums[t] = slices.Clone(values)
}

// walk returns the schema for a Go type, defining it in the bundle if it is a
// named type worth referencing.
func (s *schemaSet) walk(t reflect.Type) *schema {
	switch t {
	case timeType:
		// RFC 3339 in, RFC 3339 out — time.Time's own JSON encoding.
		return &schema{Type: schemaTypeString, Format: "date-time"}
	case rawMessageType:
		// An opaque passthrough: any JSON value, by design. Naming a shape here
		// would be inventing one the runtime does not enforce.
		return &schema{}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return s.walk(t.Elem())
	case reflect.Interface:
		return &schema{}
	case reflect.Bool:
		return &schema{Type: schemaTypeBoolean}
	case reflect.String:
		if values, ok := s.enums[t]; ok {
			return s.define(t, &schema{Type: schemaTypeString, Enum: slices.Clone(values)})
		}
		if values, ok := contractcatalog.EnumValues(t); ok {
			return s.define(t, &schema{Type: schemaTypeString, Enum: values})
		}
		return &schema{Type: schemaTypeString}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &schema{Type: schemaTypeInteger}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &schema{Type: schemaTypeInteger, Minimum: new(int64(0))}
	case reflect.Float32, reflect.Float64:
		return &schema{Type: schemaTypeNumber}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &schema{Type: schemaTypeString, ContentEncoding: "base64"}
		}
		return &schema{Type: schemaTypeArray, Items: s.walk(t.Elem())}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("contractgen: %s has a non-string map key; that is not JSON", t))
		}
		return &schema{Type: schemaTypeObject, AdditionalProps: s.walk(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return s.object(t)
		}
		return s.define(t, nil)
	default:
		panic(fmt.Sprintf("contractgen: %s (%s) has no JSON representation", t, t.Kind()))
	}
}

// define reserves a definition name, then fills it. The name is reserved BEFORE
// the body is walked so a type that reaches itself resolves to a reference
// instead of recursing forever.
func (s *schemaSet) define(t reflect.Type, body *schema) *schema {
	name := defName(t)
	if existing, ok := s.origin[name]; ok {
		if existing != t {
			panic(fmt.Sprintf("contractgen: %s and %s would both be published as %q", existing, t, name))
		}
		return &schema{Ref: refPrefix + name}
	}
	s.origin[name] = t
	if body == nil {
		body = s.object(t)
	}
	s.defs[name] = body
	return &schema{Ref: refPrefix + name}
}

// object walks a struct's fields. Optionality comes from the json tag — the
// encoder is the only authority on whether a field can be absent.
func (s *schemaSet) object(t reflect.Type) *schema {
	out := &schema{Type: schemaTypeObject, Properties: make(map[string]any)}
	for _, field := range contractshape.Fields(t) {
		node := s.walk(field.Type)
		// A value constraint on a directly-owned field narrows it in place; the
		// declared constraint and the generated Go check read the same statement.
		if node.Ref == "" {
			applyValueConstraints(node, s.valueConstraintsFor(t, field.Name))
		}
		out.Properties[field.Name] = node
		if !field.Optional {
			out.Required = append(out.Required, field.Name)
		}
	}
	// A constraint on a NESTED field cannot narrow the shared definition it lives in
	// — the rule belongs to this enclosing shape, not to every carrier of that type
	// — so it rides an allOf branch on the owner instead.
	for _, constraint := range s.values[t] {
		if !strings.Contains(constraint.Field, ".") {
			continue
		}
		branch := &schema{}
		parent, leaf := descend(branch, t, constraint.Field, false)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		narrowed := &schema{}
		applyValueConstraints(narrowed, []dispatch.FieldConstraint{constraint})
		parent.Properties[leaf] = narrowed
		out.AllOf = append(out.AllOf, branch)
	}
	if union, ok := s.unions[t]; ok {
		// A variant states the whole frame it permits, which is stricter and more
		// precise than the encoder's per-field view — so the branches own `required`
		// and the base states none.
		out.Required = nil
		out.OneOf = s.variants(t, union)
	}
	for _, rule := range s.constraints[t] {
		out.AllOf = append(out.AllOf, s.presence(t, rule))
	}
	normalize(out)
	return out
}

// variants renders a discriminated union as `oneOf`: each branch pins or narrows
// the discriminator, requires that branch's fields, and forbids every field
// belonging to another branch.
func (s *schemaSet) variants(t reflect.Type, union dispatch.UnionSpec) []*schema {
	tagValues := s.discriminatorValues(t, union)
	roots := contractshape.FieldNames(t)

	out := make([]*schema, 0, len(union.Variants)+1)
	for _, variant := range union.Variants {
		if tagValues != nil && !slices.Contains(tagValues, variant.Tag) {
			panic(fmt.Sprintf("contractgen: %s variant %q is not a value of its discriminator type", t.Name(), variant.Tag))
		}
		claimed := slices.Concat(variant.Required, variant.Optional)
		branch := &schema{
			Properties: map[string]any{union.Discriminator: &schema{Const: variant.Tag}},
			Required:   []string{union.Discriminator},
		}
		forbiddenFields := append(forbidden(t, union.Discriminator, roots, claimed), union.Forbidden...)
		constrain(branch, t, variant.Required, forbiddenFields)
		normalize(branch)
		out = append(out, branch)
	}
	if pattern := union.PatternVariant; pattern != nil {
		claimed := slices.Concat(pattern.Required, pattern.Optional)
		branch := &schema{
			Properties: map[string]any{union.Discriminator: &schema{
				Type: schemaTypeString, Pattern: pattern.TagPattern,
				TypeScriptType: pattern.TypeScriptType,
			}},
			Required: []string{union.Discriminator},
		}
		forbiddenFields := append(forbidden(t, union.Discriminator, roots, claimed), union.Forbidden...)
		constrain(branch, t, pattern.Required, forbiddenFields)
		normalize(branch)
		out = append(out, branch)
	}
	return out
}

// discriminatorValues returns the closed value set of the discriminator's own Go
// type, so a variant tag that is not one of them fails generation rather than
// producing a branch nothing can satisfy.
func (s *schemaSet) discriminatorValues(t reflect.Type, union dispatch.UnionSpec) []string {
	field, ok := contractshape.LookupField(t, union.Discriminator)
	if !ok {
		return nil
	}
	values, ok := contractcatalog.EnumValues(field.Type)
	if !ok {
		return nil
	}
	return values
}

// forbidden lists the JSON paths a variant must NOT carry: every root field no
// claim mentions, plus — inside a partially claimed nested frame — every field of
// that frame this variant does not claim.
func forbidden(t reflect.Type, discriminator string, roots, claimed []string) []string {
	claimedRoots := []string{discriminator}
	nested := make(map[string][]string)
	for _, path := range claimed {
		root, rest, dotted := strings.Cut(path, ".")
		claimedRoots = append(claimedRoots, root)
		if dotted {
			nested[root] = append(nested[root], rest)
		}
	}

	var out []string
	for _, root := range roots {
		if !slices.Contains(claimedRoots, root) {
			out = append(out, root)
		}
	}
	for root, names := range nested {
		field, ok := contractshape.LookupField(t, root)
		if !ok {
			continue
		}
		for _, name := range contractshape.FieldNames(contractshape.Deref(field.Type)) {
			if !slices.Contains(names, name) {
				out = append(out, root+"."+name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// presence renders one cross-field rule as `if/then`, or as a bare requirement
// when the rule is unconditional.
func (s *schemaSet) presence(t reflect.Type, rule dispatch.PresenceRule) *schema {
	then := &schema{}
	constrain(then, t, rule.Required, rule.Forbidden)
	normalize(then)
	if len(rule.When) == 0 {
		return then
	}
	condition := &schema{}
	for _, when := range rule.When {
		parent, leaf := descend(condition, t, when.Field, true)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		// An equals condition pins the value; a presence condition only asks that
		// the field be there, which `required` already says.
		switch when.Operator {
		case operation.OperatorEquals:
			parent.Properties[leaf] = &schema{Const: when.Value}
		case operation.OperatorPresent:
		default:
			panic(fmt.Sprintf(
				"contractgen: %s.%s uses unsupported condition operator %s",
				t.Name(),
				when.Field,
				when.Operator,
			))
		}
		parent.Required = append(parent.Required, leaf)
	}
	normalize(condition)
	return &schema{If: condition, Then: then}
}

// constrain applies required and forbidden JSON paths to a sub-schema, descending
// into nested frames for a dotted path.
func constrain(node *schema, owner reflect.Type, required, forbid []string) {
	for _, path := range required {
		// A frame that must contain something must itself be present.
		parent, leaf := descend(node, owner, path, true)
		parent.Required = append(parent.Required, leaf)
	}
	for _, path := range forbid {
		// Forbidding a nested field says nothing about whether the frame is there.
		parent, leaf := descend(node, owner, path, false)
		if parent.Properties == nil {
			parent.Properties = make(map[string]any)
		}
		parent.Properties[leaf] = false
	}
}

// descend walks a dotted path, creating the nested sub-schemas it passes through,
// and returns the sub-schema owning the last segment plus that segment's name.
func descend(node *schema, owner reflect.Type, path string, markIntermediate bool) (*schema, string) {
	segments := strings.Split(path, ".")
	current := node
	for _, segment := range segments[:len(segments)-1] {
		field, ok := contractshape.LookupField(owner, segment)
		if !ok {
			panic(fmt.Sprintf("contractgen: no JSON field %q on %s", segment, owner))
		}
		owner = contractshape.Deref(field.Type)
		if current.Properties == nil {
			current.Properties = make(map[string]any)
		}
		child, ok := current.Properties[segment].(*schema)
		if !ok {
			child = &schema{}
			current.Properties[segment] = child
		}
		if markIntermediate {
			current.Required = append(current.Required, segment)
		}
		current = child
	}
	return current, segments[len(segments)-1]
}

// normalize sorts and dedupes every `required` list in a subtree so the emitted
// artifact is byte-stable — the drift gate compares bytes, so an unstable
// generator would report drift on every run.
func normalize(node *schema) {
	if node == nil {
		return
	}
	slices.Sort(node.Required)
	node.Required = slices.Compact(node.Required)
	for _, value := range node.Properties {
		if child, ok := value.(*schema); ok {
			normalize(child)
		}
	}
	for _, child := range slices.Concat(node.OneOf, node.AnyOf, node.AllOf) {
		normalize(child)
	}
	normalize(node.Items)
	normalize(node.If)
	normalize(node.Then)
}

// defName is the published name of a Go type. A generic instantiation's
// reflect name carries the full import path of its arguments, which is neither
// readable nor a legal JSON pointer segment, so it becomes `PageOfSession`.
func defName(t reflect.Type) string {
	genericName, arguments, generic := strings.Cut(t.Name(), "[")
	if !generic {
		return genericName
	}
	var out strings.Builder
	out.WriteString(genericName)
	for argument := range strings.SplitSeq(strings.TrimSuffix(arguments, "]"), ",") {
		argument = strings.TrimPrefix(strings.TrimSpace(argument), "*")
		if index := strings.LastIndex(argument, "."); index >= 0 {
			argument = argument[index+1:]
		}
		out.WriteString("Of")
		out.WriteString(argument)
	}
	return out.String()
}

// applyValueConstraints states a declared value constraint in JSON Schema terms, so
// the schema and the generated validators refuse the same frames.
func applyValueConstraints(node *schema, constraints []dispatch.FieldConstraint) {
	for _, constraint := range constraints {
		switch constraint.Kind {
		case dispatch.ConstraintNonEmpty:
			node.MinLength = new(1)
		case dispatch.ConstraintPositive:
			node.Minimum = new(int64(1))
		case dispatch.ConstraintNonNegative:
			node.Minimum = new(int64(0))
		case dispatch.ConstraintNonEmptyItems:
			node.MinItems = new(1)
		case dispatch.ConstraintNonEmptyProperties:
			node.MinProperties = new(1)
		case dispatch.ConstraintUniqueItems:
			node.UniqueItems = true
		case dispatch.ConstraintMinItems:
			node.MinItems = new(constraint.Limit)
		case dispatch.ConstraintMaxLength:
			node.MaxLength = new(constraint.Limit)
		case dispatch.ConstraintMinimum:
			node.Minimum = new(int64(constraint.Limit))
		case dispatch.ConstraintMaximum:
			node.Maximum = new(int64(constraint.Limit))
		default:
			panic(fmt.Sprintf(
				"contractgen: unsupported value constraint %s",
				constraint.Kind,
			))
		}
	}
}
