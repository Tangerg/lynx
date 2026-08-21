package dispatch

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
)

// This file holds the shape half of the contract: which wire types are closed
// unions and which cross-field constraints they carry. The method half is
// contract.go.
//
// Go models these unions as FLAT tag-discriminated structs (one `type` field plus
// the optional fields that tag allows), which is the right wire shape but tells a
// reader nothing about which fields go with which tag. Reflection cannot recover
// that — a discriminator is not a Go concept — so it is declared here and CHECKED
// against the struct. The check is the point: a spec that names a field the struct
// does not have, or a struct field no variant accounts for, fails at startup
// instead of silently producing a schema that permits an illegal frame.

// UnionSpec declares one tag-discriminated union (contract §11.2). Literal
// variants are exact; PatternVariant is its only optional extension seam.
type UnionSpec struct {
	GoType reflect.Type
	// Discriminator is the JSON field carrying the tag. API.md §2.1 fixes it at
	// `type` for every first-party union, with no exceptions.
	Discriminator string
	Variants      []VariantSpec
	// PatternVariant keeps a union extensible without weakening its known variants
	// to `type: string`. Its tag pattern must be disjoint from every literal tag;
	// TypeScriptType is the corresponding narrow string type emitted by the SDK
	// (for example `plugin:${string}/${string}`).
	PatternVariant *PatternVariantSpec
	// Forbidden names wire members that no variant may carry even though the Go
	// shape no longer has them. This is for protocol-level negative invariants
	// under an otherwise open object envelope, such as rejecting a removed
	// sender-controlled reliability assertion. It never enables decoding an old
	// shape.
	Forbidden []string
}

// PatternVariantSpec is the one namespaced extension branch of an otherwise
// literal-tagged union. Required and Optional have the same whole-frame meaning
// as [VariantSpec].
type PatternVariantSpec struct {
	TagPattern     string
	TypeScriptType string
	Required       []string
	Optional       []string
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
	When      []operation.FieldCondition
	Required  []string
	Forbidden []string
}

// CarriedSpec declares a wire type the method graph cannot reach.
//
// The artifact walk starts from the registered methods, so a delivery-owned shape
// that rides somewhere else is invisible to it; `params._meta`, for example, is
// stripped before typed decoding. Concrete tool results are published from
// toolset's presentation contracts instead of being restated here.
//
// Carrier says WHERE it rides, in wire terms. A bare list of types would publish
// the shapes without answering the only question a reader has about them.
type CarriedSpec struct {
	Carrier string
	GoType  reflect.Type
}

// NotificationSpec declares one downstream JSON-RPC notification and the exact
// params shape it carries. Notifications are not callable methods, so they belong
// to the wire-shape registry rather than the request router.
type NotificationSpec struct {
	Name       string
	ParamsType reflect.Type
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
	// ConstraintNonNegative rejects negative numeric values while preserving zero
	// as the wire spelling of an omitted/unbounded limit.
	ConstraintNonNegative
	// ConstraintNonEmptyItems rejects an empty array. An optional narrowing set
	// already uses absence for "no narrower scope", while a required set names the
	// minimum recovery or transaction unit. An empty third spelling has no useful
	// meaning in either direction.
	ConstraintNonEmptyItems
	// ConstraintNonEmptyProperties rejects an empty object map. Secret-map
	// replacement uses omission to preserve and a clear variant to remove, so an
	// empty set value would be a third, ambiguous spelling of clear.
	ConstraintNonEmptyProperties
	// ConstraintUniqueItems rejects a repeated element. A filter is a set, and a
	// value listed twice means the caller believes it is asking something a set
	// cannot express.
	ConstraintUniqueItems
	// ConstraintMinItems rejects an array shorter than FieldConstraint.Limit.
	// Unlike ConstraintNonEmptyItems, its bound is part of the contract rather
	// than the special distinction between omission and an explicitly empty set.
	ConstraintMinItems
	// ConstraintMaxLength rejects a string containing more Unicode code points
	// than FieldConstraint.Limit, matching JSON Schema's length semantics.
	ConstraintMaxLength
	// ConstraintMinimum rejects a number smaller than FieldConstraint.Limit.
	// It is inclusive, matching JSON Schema's minimum keyword.
	ConstraintMinimum
	// ConstraintMaximum rejects a number greater than FieldConstraint.Limit.
	// It is inclusive, matching JSON Schema's maximum keyword.
	ConstraintMaximum
)

func (k ConstraintKind) String() string {
	switch k {
	case ConstraintNonEmpty:
		return "nonEmpty"
	case ConstraintPositive:
		return "positive"
	case ConstraintNonNegative:
		return "nonNegative"
	case ConstraintNonEmptyItems:
		return "nonEmptyItems"
	case ConstraintNonEmptyProperties:
		return "nonEmptyProperties"
	case ConstraintUniqueItems:
		return "uniqueItems"
	case ConstraintMinItems:
		return "minItems"
	case ConstraintMaxLength:
		return "maxLength"
	case ConstraintMinimum:
		return "minimum"
	case ConstraintMaximum:
		return "maximum"
	default:
		return fmt.Sprintf("ConstraintKind(%d)", k)
	}
}

// FieldConstraint is one field's value constraint. Field is a dotted JSON path.
type FieldConstraint struct {
	Field string
	Kind  ConstraintKind
	Limit int
}

func (c FieldConstraint) String() string {
	switch c.Kind {
	case ConstraintMinItems, ConstraintMaxLength, ConstraintMinimum, ConstraintMaximum:
		return fmt.Sprintf("%s(%d)", c.Kind, c.Limit)
	default:
		return c.Kind.String()
	}
}

// FieldConstraintSpec declares the value constraints of one wire shape.
//
// These are the checks reflection cannot see: that a string must be non-empty,
// that a number must exceed or not fall below zero. Closed-enum membership is NOT declared here —
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
	unions        []UnionSpec
	constraints   []ObjectConstraintSpec
	carried       []CarriedSpec
	notifications []NotificationSpec
	values        []FieldConstraintSpec
}

func (s *Shapes) Unions() []UnionSpec {
	out := make([]UnionSpec, len(s.unions))
	for index, spec := range s.unions {
		out[index] = cloneUnionSpec(spec)
	}
	return out
}

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
		byType[spec.GoType] = cloneObjectConstraintSpec(spec)
	}
	out := make([]ObjectConstraintSpec, 0, len(s.constraints))
	for _, stored := range s.constraints {
		spec := cloneObjectConstraintSpec(stored)
		for _, embedded := range contractshape.Embeds(spec.GoType) {
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
func (s *Shapes) Carried() []CarriedSpec { return slices.Clone(s.carried) }
func (s *Shapes) Notifications() []NotificationSpec {
	return slices.Clone(s.notifications)
}
func (s *Shapes) ValueConstraints() []FieldConstraintSpec {
	out := make([]FieldConstraintSpec, len(s.values))
	for index, spec := range s.values {
		spec.Constraints = slices.Clone(spec.Constraints)
		out[index] = spec
	}
	return out
}

func cloneUnionSpec(spec UnionSpec) UnionSpec {
	spec.Forbidden = slices.Clone(spec.Forbidden)
	spec.Variants = slices.Clone(spec.Variants)
	for index := range spec.Variants {
		spec.Variants[index].Required = slices.Clone(spec.Variants[index].Required)
		spec.Variants[index].Optional = slices.Clone(spec.Variants[index].Optional)
	}
	if spec.PatternVariant != nil {
		pattern := *spec.PatternVariant
		pattern.Required = slices.Clone(pattern.Required)
		pattern.Optional = slices.Clone(pattern.Optional)
		spec.PatternVariant = &pattern
	}
	return spec
}

func cloneObjectConstraintSpec(spec ObjectConstraintSpec) ObjectConstraintSpec {
	spec.Rules = slices.Clone(spec.Rules)
	for index := range spec.Rules {
		spec.Rules[index].When = slices.Clone(spec.Rules[index].When)
		spec.Rules[index].Required = slices.Clone(spec.Rules[index].Required)
		spec.Rules[index].Forbidden = slices.Clone(spec.Rules[index].Forbidden)
	}
	return spec
}

func (s *Shapes) union(spec UnionSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid union spec: " + err.Error())
	}
	if slices.ContainsFunc(s.unions, func(existing UnionSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: union spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	s.unions = append(s.unions, cloneUnionSpec(spec))
}

func (s *Shapes) constraint(spec ObjectConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid object constraint spec: " + err.Error())
	}
	if slices.ContainsFunc(s.constraints, func(existing ObjectConstraintSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: object constraint spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	s.constraints = append(s.constraints, cloneObjectConstraintSpec(spec))
}

func (s *Shapes) valueConstraint(spec FieldConstraintSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid value constraint spec: " + err.Error())
	}
	if slices.ContainsFunc(s.values, func(existing FieldConstraintSpec) bool {
		return existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: value constraint spec for %s is registered twice",
			spec.GoType.Name(),
		))
	}
	spec.Constraints = slices.Clone(spec.Constraints)
	s.values = append(s.values, spec)
}

func (s *Shapes) carriedShape(spec CarriedSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid carried shape spec: " + err.Error())
	}
	if slices.ContainsFunc(s.carried, func(existing CarriedSpec) bool {
		return existing.Carrier == spec.Carrier && existing.GoType == spec.GoType
	}) {
		panic(fmt.Sprintf(
			"dispatch: carried shape %s on %q is registered twice",
			spec.GoType,
			spec.Carrier,
		))
	}
	s.carried = append(s.carried, spec)
}

func (s *Shapes) notification(spec NotificationSpec) {
	if err := spec.validate(); err != nil {
		panic("dispatch: invalid notification spec: " + err.Error())
	}
	if slices.ContainsFunc(s.notifications, func(existing NotificationSpec) bool {
		return existing.Name == spec.Name
	}) {
		panic(fmt.Sprintf("dispatch: notification %q is registered twice", spec.Name))
	}
	s.notifications = append(s.notifications, spec)
}

// validate checks a union spec against the struct it describes.
func (u UnionSpec) validate() error {
	if u.GoType == nil || u.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("union spec needs a struct type, got %v", u.GoType)
	}
	validation := unionValidation{
		spec: u, name: u.GoType.Name(),
		accounted: []string{u.Discriminator},
		tags:      make(map[string]bool, len(u.Variants)),
	}
	if err := validation.validateDiscriminator(); err != nil {
		return err
	}
	if err := validation.validateForbiddenFields(); err != nil {
		return err
	}
	for _, variant := range u.Variants {
		if err := validation.validateLiteralVariant(variant); err != nil {
			return err
		}
	}
	if err := validation.validatePatternVariant(); err != nil {
		return err
	}
	return validation.validateCoverage()
}

type unionValidation struct {
	spec      UnionSpec
	name      string
	accounted []string
	tags      map[string]bool
}

func (validation *unionValidation) validateDiscriminator() error {
	u := validation.spec
	if u.Discriminator != "type" {
		return fmt.Errorf(
			"%s: discriminator is %q — API.md §2.1 fixes it at \"type\"",
			validation.name,
			u.Discriminator,
		)
	}
	if err := contractshape.HasPath(u.GoType, u.Discriminator); err != nil {
		return fmt.Errorf("%s: %w", validation.name, err)
	}
	if len(u.Variants) == 0 {
		return fmt.Errorf("%s: a union with no literal variants describes nothing", validation.name)
	}
	return nil
}

func (validation *unionValidation) validateForbiddenFields() error {
	u := validation.spec
	for index, field := range u.Forbidden {
		switch {
		case field == "":
			return fmt.Errorf("%s: forbidden field %d has no name", validation.name, index)
		case strings.Contains(field, "."):
			return fmt.Errorf(
				"%s: forbidden field %q must be a top-level JSON member",
				validation.name,
				field,
			)
		case slices.Contains(u.Forbidden[:index], field):
			return fmt.Errorf("%s: forbidden field %q is declared twice", validation.name, field)
		case slices.Contains(contractshape.FieldNames(u.GoType), field):
			return fmt.Errorf(
				"%s: forbidden field %q still exists on the Go wire shape",
				validation.name,
				field,
			)
		}
	}
	return nil
}

func (validation *unionValidation) validateLiteralVariant(variant VariantSpec) error {
	if variant.Tag == "" {
		return fmt.Errorf("%s: a variant needs a tag", validation.name)
	}
	if validation.tags[variant.Tag] {
		return fmt.Errorf("%s: variant %q is declared twice", validation.name, variant.Tag)
	}
	validation.tags[variant.Tag] = true
	return validation.claimFields(
		fmt.Sprintf("%s variant %q", validation.name, variant.Tag),
		variant.Required,
		variant.Optional,
	)
}

func (validation *unionValidation) validatePatternVariant() error {
	pattern := validation.spec.PatternVariant
	if pattern == nil {
		return nil
	}
	compiled, err := regexp.Compile(pattern.TagPattern)
	switch {
	case pattern.TagPattern == "":
		return fmt.Errorf("%s: pattern variant needs a tag pattern", validation.name)
	case err != nil:
		return fmt.Errorf(
			"%s: invalid pattern variant tag %q: %w",
			validation.name,
			pattern.TagPattern,
			err,
		)
	case pattern.TypeScriptType == "":
		return fmt.Errorf("%s: pattern variant needs a TypeScript type", validation.name)
	}
	for tag := range validation.tags {
		if compiled.MatchString(tag) {
			return fmt.Errorf(
				"%s: pattern variant also matches literal tag %q",
				validation.name,
				tag,
			)
		}
	}
	return validation.claimFields(
		validation.name+" pattern variant",
		pattern.Required,
		pattern.Optional,
	)
}

func (validation *unionValidation) claimFields(
	owner string,
	required []string,
	optional []string,
) error {
	for index, field := range required {
		if slices.Contains(required[:index], field) {
			return fmt.Errorf("%s: required field %q is declared twice", owner, field)
		}
	}
	for index, field := range optional {
		switch {
		case slices.Contains(optional[:index], field):
			return fmt.Errorf("%s: optional field %q is declared twice", owner, field)
		case slices.Contains(required, field):
			return fmt.Errorf("%s: field %q cannot be both required and optional", owner, field)
		}
	}
	for _, field := range slices.Concat(required, optional) {
		if err := contractshape.HasPath(validation.spec.GoType, field); err != nil {
			return fmt.Errorf("%s: %w", owner, err)
		}
		// A nested declaration accounts for the frame that holds it: claiming
		// `payload.tool` claims `payload`.
		root := strings.Split(field, ".")[0]
		if !slices.Contains(validation.accounted, root) {
			validation.accounted = append(validation.accounted, root)
		}
	}
	return nil
}

func (validation *unionValidation) validateCoverage() error {
	// The drift that actually happens: a field is added to the struct and no
	// variant claims it, so the generated schema would allow it under every tag.
	for _, field := range contractshape.FieldNames(validation.spec.GoType) {
		if !slices.Contains(validation.accounted, field) {
			return fmt.Errorf(
				"%s: field %q belongs to no variant — every union field must name its tag",
				validation.name,
				field,
			)
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
		for conditionIndex, condition := range rule.When {
			if slices.Contains(rule.When[:conditionIndex], condition) {
				return fmt.Errorf(
					"%s rule %d: condition for field %q with operator %s is declared twice",
					name, index, condition.Field, condition.Operator,
				)
			}
			if err := operation.ValidateFieldCondition(
				fmt.Sprintf("%s rule %d", name, index),
				o.GoType,
				condition,
			); err != nil {
				return err
			}
		}
		for fieldIndex, field := range rule.Required {
			if slices.Contains(rule.Required[:fieldIndex], field) {
				return fmt.Errorf(
					"%s rule %d: required field %q is declared twice",
					name, index, field,
				)
			}
		}
		for fieldIndex, field := range rule.Forbidden {
			switch {
			case slices.Contains(rule.Forbidden[:fieldIndex], field):
				return fmt.Errorf(
					"%s rule %d: forbidden field %q is declared twice",
					name, index, field,
				)
			case slices.Contains(rule.Required, field):
				return fmt.Errorf(
					"%s rule %d: field %q cannot be both required and forbidden",
					name, index, field,
				)
			}
		}
		for _, field := range slices.Concat(rule.Required, rule.Forbidden) {
			if err := contractshape.HasPath(o.GoType, field); err != nil {
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
	for index, constraint := range f.Constraints {
		if slices.ContainsFunc(f.Constraints[:index], func(previous FieldConstraint) bool {
			return previous.Field == constraint.Field && previous.Kind == constraint.Kind
		}) {
			return fmt.Errorf(
				"%s.%s declares constraint %s twice",
				name, constraint.Field, constraint.Kind,
			)
		}
		if err := validateFieldConstraint(name, f.GoType, constraint); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldConstraint(owner string, shape reflect.Type, constraint FieldConstraint) error {
	_, leaf, ok := contractshape.GoPath(shape, constraint.Field)
	if !ok {
		return fmt.Errorf("%s: no JSON field %q", owner, constraint.Field)
	}
	if err := validateConstraintLimit(owner, constraint); err != nil {
		return err
	}
	return validateConstraintTarget(owner, leaf.Type, constraint)
}

func validateConstraintLimit(owner string, constraint FieldConstraint) error {
	acceptsLimit := constraint.Kind == ConstraintMinItems ||
		constraint.Kind == ConstraintMaxLength ||
		constraint.Kind == ConstraintMinimum ||
		constraint.Kind == ConstraintMaximum
	if acceptsLimit && constraint.Limit <= 0 {
		return fmt.Errorf(
			"%s.%s constraint %s needs a positive limit",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	if !acceptsLimit && constraint.Limit != 0 {
		return fmt.Errorf(
			"%s.%s constraint %s does not accept a limit",
			owner,
			constraint.Field,
			constraint.Kind,
		)
	}
	return nil
}

func validateConstraintTarget(owner string, declaredType reflect.Type, constraint FieldConstraint) error {
	valueType := declaredType
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	kind := valueType.Kind()
	switch constraint.Kind {
	case ConstraintNonEmpty:
		if kind != reflect.String {
			return fmt.Errorf("%s.%s is %s; only a string can be non-empty", owner, constraint.Field, kind)
		}
	case ConstraintPositive:
		if kind != reflect.Uint64 && kind != reflect.Int && kind != reflect.Int64 {
			return fmt.Errorf("%s.%s is %s; only a number can be positive", owner, constraint.Field, kind)
		}
	case ConstraintNonNegative:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can be non-negative", owner, constraint.Field, kind)
		}
	case ConstraintNonEmptyItems, ConstraintUniqueItems, ConstraintMinItems:
		if kind != reflect.Slice {
			return fmt.Errorf("%s.%s is %s; only an array has items", owner, constraint.Field, declaredType.Kind())
		}
	case ConstraintMaxLength:
		if kind != reflect.String {
			return fmt.Errorf("%s.%s is %s; only a string has a length", owner, constraint.Field, kind)
		}
	case ConstraintMinimum:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can have a minimum", owner, constraint.Field, kind)
		}
	case ConstraintMaximum:
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Float64 {
			return fmt.Errorf("%s.%s is %s; only a number can have a maximum", owner, constraint.Field, kind)
		}
	case ConstraintNonEmptyProperties:
		if kind != reflect.Map {
			return fmt.Errorf("%s.%s is %s; only an object map has properties", owner, constraint.Field, declaredType.Kind())
		}
	default:
		return fmt.Errorf(
			"%s.%s has invalid constraint kind %s; expected %s, %s, %s, %s, %s, %s, %s, %s, %s or %s",
			owner,
			constraint.Field,
			constraint.Kind,
			ConstraintNonEmpty,
			ConstraintPositive,
			ConstraintNonNegative,
			ConstraintNonEmptyItems,
			ConstraintNonEmptyProperties,
			ConstraintUniqueItems,
			ConstraintMinItems,
			ConstraintMaxLength,
			ConstraintMinimum,
			ConstraintMaximum,
		)
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

func (n NotificationSpec) validate() error {
	switch {
	case n.Name == "":
		return errors.New("notification spec needs a method name")
	case n.ParamsType == nil:
		return fmt.Errorf("notification %q has no params type", n.Name)
	case n.ParamsType.Kind() != reflect.Struct || n.ParamsType.Name() == "":
		return fmt.Errorf("notification %q params must be a named struct, got %v", n.Name, n.ParamsType)
	case !strings.HasPrefix(n.Name, "notifications."):
		return fmt.Errorf("notification %q must use the notifications namespace", n.Name)
	}
	return nil
}
