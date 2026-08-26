package main

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/contractcatalog"
	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
	runtimehttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// The TypeScript wire types are emitted from the SCHEMA tree, not from a second
// walk of the Go types.
//
// That is the whole design: two independent walks would be two authors of the same
// shape, and contract §11.4 gate 6 asks that the schema and the TS types agree.
// Deriving one from the other makes agreement structural — there is no path by
// which a variant can be forbidden in the schema and allowed in TypeScript.
//
// What TypeScript cannot express is left out on purpose: a presence rule
// (`if/then`) is a runtime check, not a type, and the generated validator is where
// it lands. A type system that silently drops half a constraint is worse than one
// that says which half it holds.

// tsFileName is the generated shape module. Its name makes generated ownership
// explicit to every consumer that vendors the binding.
const tsFileName = "wire.generated.ts"

type tsEmitter struct {
	tsTypes
	out strings.Builder
}

// tsTypes answers "how does TypeScript spell this?" for a schema node or a Go
// type. Both generated modules ask it — the shapes module to declare a type, the
// methods module to name a method's frames — and two answers to that question
// would be two spellings of one shape.
type tsTypes struct {
	set *schemaSet
}

func newTypeScript(set *schemaSet, notifications []string) string {
	emitter := &tsEmitter{tsTypes: tsTypes{set: set}}
	emitter.header()
	emitter.protocolVersion()
	emitter.httpEndpoints()
	emitter.notifications(notifications)

	names := slices.Sorted(maps.Keys(set.defs))
	generics := genericBases(set)
	for genericName, instantiations := range generics {
		emitter.generic(genericName, instantiations)
	}
	for _, name := range names {
		if _, aliased := generics[genericBaseOf(set.origin[name])]; aliased && isInstantiation(set.origin[name]) {
			emitter.alias(name, set.origin[name])
			continue
		}
		emitter.define(name, set.defs[name])
	}
	emitter.enumValues(names)
	emitter.runEventReliability()
	return emitter.out.String()
}

// httpEndpoints publishes transport locations from the same Delivery registry
// the HTTP router uses. A vendored client never has to restate /v2 paths.
func (t *tsEmitter) httpEndpoints() {
	t.line("// HTTP entrypoints implemented by this runtime build.")
	t.line("export const HTTP_ENDPOINTS = {")
	contract := runtimehttp.Contract()
	for _, endpoint := range contract.Endpoints {
		t.line("  %s: {", propertyKey(endpoint.Name))
		t.line("    kind: %s,", strconv.Quote(string(endpoint.Kind)))
		t.line("    method: %s,", strconv.Quote(endpoint.Method))
		t.line("    path: %s,", strconv.Quote(endpoint.Path))
		t.line("    authentication: %s,", strconv.Quote(string(endpoint.Authentication)))
		statuses := make([]string, 0, len(endpoint.ResponseStatuses))
		for _, status := range endpoint.ResponseStatuses {
			statuses = append(statuses, strconv.Itoa(status))
		}
		t.line("    responseStatuses: [%s],", strings.Join(statuses, ", "))
		t.line("  },")
	}
	t.line("} as const;")
	t.line("")
	t.line("// Flat-JSON responses returned outside the JSON-RPC envelope.")
	t.line("export interface HTTPSidecarResponses {")
	for _, endpoint := range contract.Endpoints {
		if endpoint.Kind != runtimehttp.EndpointKindSidecar || endpoint.ResponseType == nil {
			continue
		}
		t.line("  %s: %s;", propertyKey(endpoint.Name), defName(endpoint.ResponseType))
	}
	t.line("}")
	t.line("")
	t.line("export type HTTPSidecarEndpointName = keyof HTTPSidecarResponses;")
	t.line("")
}

// notifications emits the downstream method names.
//
// A client subscribes to these by name — nothing inbound routes to them — so the
// name is the whole contract, and a client that spells it itself is a second author
// of the method surface. The constant name is derived mechanically so no naming
// scheme has to be invented per notification.
func (t *tsEmitter) notifications(methods []string) {
	t.line("// The methods the runtime sends downstream. A client only ever subscribes.")
	for _, method := range methods {
		t.line("export const %s = %s;", screamingSnake(method), strconv.Quote(method))
	}
	t.line("")
}

// screamingSnake turns a dotted method name into a TypeScript constant name:
// notifications.run.event becomes NOTIFICATIONS_RUN_EVENT.
func screamingSnake(name string) string {
	var out strings.Builder
	for index, letter := range name {
		switch {
		case letter == '.':
			out.WriteByte('_')
		case letter >= 'A' && letter <= 'Z' && index > 0:
			out.WriteByte('_')
			out.WriteRune(letter)
		default:
			out.WriteRune(letter - lowerToUpper(letter))
		}
	}
	return out.String()
}

func lowerToUpper(letter rune) rune {
	if letter >= 'a' && letter <= 'z' {
		return 'a' - 'A'
	}
	return 0
}

// enumValues emits the runtime face of the enums.
//
// A TypeScript union type does not exist at runtime, so a client that has to
// ITERATE a closed value set — check its sample coverage, validate an incoming
// tag — cannot get one from the types alone. One record keyed by type name avoids
// inventing a naming scheme for fifty-one separate constants, and `as const` keeps
// each list narrowed to its own literals.
func (t *tsEmitter) enumValues(names []string) {
	t.line("// The closed value sets, as data: a union type does not exist at runtime.")
	t.line("export const WIRE_ENUMS = {")
	for _, name := range names {
		node := t.set.defs[name]
		if len(node.Enum) == 0 {
			continue
		}
		t.line("  %s: [%s],", name, strings.Join(quoteAll(node.Enum), ", "))
	}
	t.line("} as const;")
}

func (t *tsEmitter) runEventReliability() {
	values, ok := contractcatalog.EnumValues(reflect.TypeFor[protocol.StreamEventType]())
	if !ok {
		panic("contractgen: StreamEventType is not a registered wire enum")
	}
	t.line("")
	t.line("/** Reliability is owned by event type; a frame cannot promote itself. */")
	t.line("export type RunEventReliability = \"authoritative\" | \"ephemeral\";")
	t.line("export const RUN_EVENT_RELIABILITY = {")
	for _, value := range values {
		reliability := "ephemeral"
		if (protocol.StreamEvent{Type: protocol.StreamEventType(value)}).Authoritative() {
			reliability = "authoritative"
		}
		t.line("  %s: %s,", strconv.Quote(value), strconv.Quote(reliability))
	}
	t.line("} as const satisfies Record<StreamEventType, RunEventReliability>;")
	t.line("")
	t.line("export function runEventReliability(value: unknown): RunEventReliability | undefined {")
	t.line("  if (typeof value !== \"string\") return undefined;")
	t.line("  return (RUN_EVENT_RELIABILITY as Partial<Record<string, RunEventReliability>>)[value];")
	t.line("}")
}

func (t *tsEmitter) header() {
	t.line("// Code generated by cmd/contractgen. DO NOT EDIT.")
	t.line("//")
	t.line("// The wire types of the Lyra Runtime Protocol, projected from the Contract")
	t.line("// Registry. Prose lives with the Go types and protocol documentation; this file")
	t.line("// publishes the shapes without writing into any consuming client module.")
	t.line("//")
	t.line("// Cross-field rules — a finished run carries an outcome, an interrupt outcome")
	t.line("// carries no result — are NOT here: TypeScript has no way to state them. They are")
	t.line("// in the generated validator and in schema.json.")
	t.line("")
}

// protocolVersion emits the version the client states in request metadata.
//
// The runtime refuses a request naming a version outside the range it serves, so a
// client that spells the date itself is a second author of the negotiation — and
// the failure is a rejected handshake, not a type error. Projecting the constant
// makes "the client sends what this build serves" true by construction.
func (t *tsEmitter) protocolVersion() {
	t.line("// The wire version this runtime serves; a client states it in request metadata.")
	t.line("export const PROTOCOL_VERSION = %s;", strconv.Quote(protocol.ProtocolVersion))
	t.line("")
}

// generic reconstructs a Go generic as a TypeScript generic.
//
// The walk sees only instantiations — `Page[Session]`, `Page[Model]` — because Go
// reflection has no view of an uninstantiated type. Emitting one interface per
// instantiation would publish nineteen copies of the same two fields and break
// generic helpers in consuming clients, so the shape is recovered by substituting
// the type argument out of one instantiation, and every OTHER instantiation must
// reproduce it exactly or generation fails.
func (t *tsEmitter) generic(genericName string, instantiations []string) {
	var body, source string
	for _, name := range instantiations {
		argument := t.typeName(typeArgumentOf(t.set.origin[name]))
		rendered := t.objectBody(t.set.defs[name], argument)
		if body == "" {
			body, source = rendered, name
			continue
		}
		if rendered != body {
			panic(fmt.Sprintf("contractgen: %s and %s are the same generic with different shapes:\n%s\n%s",
				source, name, body, rendered))
		}
	}
	if !strings.Contains(body, "T") {
		panic(fmt.Sprintf("contractgen: %s uses its type parameter nowhere", genericName))
	}
	t.line("export interface %s<T> {", genericName)
	t.out.WriteString(body)
	t.line("}")
	t.line("")
}

func (t *tsEmitter) alias(name string, shape reflect.Type) {
	t.line("export type %s = %s<%s>;", name, genericBaseOf(shape), t.typeName(typeArgumentOf(shape)))
	t.line("")
}

func (t *tsEmitter) define(name string, node *schema) {
	switch {
	case len(node.Enum) > 0:
		t.line("export type %s = %s;", name, unionOf(quoteAll(node.Enum)))
	case len(node.OneOf) > 0:
		t.line("export type %s =", name)
		for _, branch := range node.OneOf {
			t.line("  | %s", t.branch(node, branch))
		}
		t.trimTrailingNewline()
		t.out.WriteString(";\n")
	default:
		t.line("export interface %s {", name)
		t.out.WriteString(t.objectBody(node, ""))
		t.line("}")
	}
	t.line("")
}

// objectBody renders an interface's fields. substitute, when set, is the type name
// to replace with the generic parameter T.
func (t *tsEmitter) objectBody(node *schema, substitute string) string {
	var out strings.Builder
	for _, name := range slices.Sorted(maps.Keys(node.Properties)) {
		child, ok := node.Properties[name].(*schema)
		if !ok {
			continue
		}
		rendered := t.typeOf(child)
		if substitute != "" {
			rendered = substituteType(rendered, substitute)
		}
		fmt.Fprintf(&out, "  %s%s: %s;\n", propertyKey(name), optionalMark(node, name), rendered)
	}
	return out.String()
}

// branch renders one variant of a discriminated union: the shared fields this tag permits,
// with the discriminator pinned and the other tags' fields gone.
func (t *tsEmitter) branch(unionSchema, variantSchema *schema) string {
	var fields []string
	for _, name := range discriminatorFirst(unionSchema, variantSchema) {
		unionField, ok := unionSchema.Properties[name].(*schema)
		if !ok {
			continue
		}
		override, mentioned := variantSchema.Properties[name]
		if excluded, ok := override.(bool); mentioned && ok && !excluded {
			// The boolean schema `false`: this tag may not carry the field at all.
			continue
		}
		rendered := t.typeOf(unionField)
		if narrowing, ok := override.(*schema); mentioned && ok {
			rendered = t.narrow(unionField, narrowing)
		}
		fields = append(fields, fmt.Sprintf("%s%s: %s", propertyKey(name), optionalMark(variantSchema, name), rendered))
	}
	return "{ " + strings.Join(fields, "; ") + " }"
}

// narrow applies a branch's constraint on a nested frame: the tag pins the
// discriminator to a literal, or claims some of the frame's fields and forbids the
// rest.
func (t *tsEmitter) narrow(baseChild, narrowing *schema) string {
	if narrowing.Const != "" {
		return strconv.Quote(narrowing.Const)
	}
	if narrowing.TypeScriptType != "" {
		return narrowing.TypeScriptType
	}
	frame := t.resolve(baseChild)
	if frame == nil {
		return t.typeOf(baseChild)
	}
	var fields []string
	for _, name := range slices.Sorted(maps.Keys(frame.Properties)) {
		child, ok := frame.Properties[name].(*schema)
		if !ok {
			continue
		}
		if excluded, ok := narrowing.Properties[name].(bool); ok && !excluded {
			continue
		}
		fields = append(fields, fmt.Sprintf("%s%s: %s", propertyKey(name), optionalMark(narrowing, name), t.typeOf(child)))
	}
	return "{ " + strings.Join(fields, "; ") + " }"
}

// resolve follows a reference to the definition it names.
func (t tsTypes) resolve(node *schema) *schema {
	name, ok := strings.CutPrefix(node.Ref, refPrefix)
	if !ok {
		return nil
	}
	return t.set.defs[name]
}

// typeOf renders a schema node as a TypeScript type expression.
func (t tsTypes) typeOf(node *schema) string {
	if node.TypeScriptType != "" {
		return node.TypeScriptType
	}
	if node.Ref != "" {
		name, _ := strings.CutPrefix(node.Ref, refPrefix)
		return name
	}
	if len(node.AnyOf) > 0 {
		parts := make([]string, 0, len(node.AnyOf))
		for _, member := range node.AnyOf {
			parts = append(parts, t.typeOf(member))
		}
		return unionOf(parts)
	}
	if len(node.Enum) > 0 {
		return unionOf(quoteAll(node.Enum))
	}
	names := typeKeywords(node.Type)
	if len(names) == 0 {
		// An empty schema is any JSON value. `unknown` forces the reader to narrow,
		// which is exactly right for an opaque passthrough.
		return "unknown"
	}
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, t.keyword(name, node))
	}
	return unionOf(parts)
}

func (t tsTypes) keyword(name schemaType, node *schema) string {
	switch name {
	case schemaTypeString:
		return "string"
	case schemaTypeInteger, schemaTypeNumber:
		return "number"
	case schemaTypeBoolean:
		return "boolean"
	case schemaTypeNull:
		return "null"
	case schemaTypeArray:
		if node.Items == nil {
			return "unknown[]"
		}
		element := t.typeOf(node.Items)
		if strings.Contains(element, "|") {
			return "(" + element + ")[]"
		}
		return element + "[]"
	case schemaTypeObject:
		if child, ok := node.AdditionalProps.(*schema); ok {
			return "Record<string, " + t.typeOf(child) + ">"
		}
		if len(node.Properties) == 0 {
			return "Record<string, never>"
		}
		var fields []string
		for _, property := range slices.Sorted(maps.Keys(node.Properties)) {
			child, ok := node.Properties[property].(*schema)
			if !ok {
				continue
			}
			fields = append(fields, fmt.Sprintf("%s%s: %s", propertyKey(property), optionalMark(node, property), t.typeOf(child)))
		}
		return "{ " + strings.Join(fields, "; ") + " }"
	default:
		panic("contractgen: no TypeScript type for JSON type " + string(name))
	}
}

// typeName is the published name of a Go type, as TypeScript spells it.
func (t tsTypes) typeName(shape reflect.Type) string {
	if shape == nil {
		return "unknown"
	}
	if name := defName(shape); name != "" {
		return name
	}
	return t.typeOf(t.set.walk(shape))
}

func (t *tsEmitter) line(format string, arguments ...any) {
	fmt.Fprintf(&t.out, format+"\n", arguments...)
}

func (t *tsEmitter) trimTrailingNewline() {
	text := strings.TrimSuffix(t.out.String(), "\n")
	t.out.Reset()
	t.out.WriteString(text)
}

// genericBases groups the instantiations of each generic Go type by generic name.
func genericBases(set *schemaSet) map[string][]string {
	out := make(map[string][]string)
	for name, t := range set.origin {
		if !isInstantiation(t) {
			continue
		}
		genericName := genericBaseOf(t)
		out[genericName] = append(out[genericName], name)
	}
	for genericName := range out {
		slices.Sort(out[genericName])
	}
	return out
}

func isInstantiation(t reflect.Type) bool {
	return t != nil && strings.Contains(t.Name(), "[")
}

func genericBaseOf(t reflect.Type) string {
	if t == nil {
		return ""
	}
	genericName, _, _ := strings.Cut(t.Name(), "[")
	return genericName
}

// typeArgumentOf returns the single type argument of a generic instantiation, found
// by matching the argument's own name against the struct's fields — reflection
// hands back instantiated field types, so the argument is recovered from them.
func typeArgumentOf(t reflect.Type) reflect.Type {
	_, arguments, generic := strings.Cut(t.Name(), "[")
	if !generic {
		return nil
	}
	wanted := strings.TrimSuffix(arguments, "]")
	if index := strings.LastIndex(wanted, "."); index >= 0 {
		wanted = wanted[index+1:]
	}
	for _, field := range contractshape.Fields(t) {
		if candidate := contractshape.Deref(field.Type); candidate.Name() == wanted {
			return candidate
		}
	}
	return nil
}

// substituteType replaces a concrete type name with the generic parameter. It
// matches whole identifiers so `Session` in `SessionRef` is left alone.
func substituteType(rendered, concrete string) string {
	return regexp.MustCompile(`\b`+regexp.QuoteMeta(concrete)+`\b`).ReplaceAllString(rendered, "T")
}

func optionalMark(owner *schema, property string) string {
	if slices.Contains(owner.Required, property) {
		return ""
	}
	return "?"
}

// typeKeywords keeps the type-emission loop uniform for an optional keyword.
func typeKeywords(value schemaType) []schemaType {
	if value == "" {
		return nil
	}
	return []schemaType{value}
}

var identifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func propertyKey(name string) string {
	if identifier.MatchString(name) {
		return name
	}
	return strconv.Quote(name)
}

func quoteAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Quote(value))
	}
	return out
}

func unionOf(parts []string) string { return strings.Join(parts, " | ") }

// discriminatorFirst orders a variant's fields with the tag in front. A reader
// scanning a union wants to see which case each line is before its payload, and
// the tag is recognizable without being named: it is the field the branch pins to
// a literal.
func discriminatorFirst(unionSchema, variantSchema *schema) []string {
	names := slices.Sorted(maps.Keys(unionSchema.Properties))
	slices.SortStableFunc(names, func(a, b string) int {
		return boolCompare(isPinned(variantSchema, b), isPinned(variantSchema, a))
	})
	return names
}

func isPinned(branch *schema, name string) bool {
	pinned, ok := branch.Properties[name].(*schema)
	return ok && pinned.Const != ""
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
