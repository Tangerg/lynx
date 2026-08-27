package main

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// The method surface, as TypeScript reads it: which methods exist, what each one
// carries, and what has to be negotiated before one will run.
//
// This is the half of contract §11.3's fifth artifact class that has a reader. Go
// needs no generated method constants — `server` and `dispatch` are the same ring
// and the Registry itself is their shared source, so a second table of
// `MethodRunsStart = "runs.start"` that dispatch does not consume would BE the
// second author it was meant to remove. TypeScript cannot read the Registry at all,
// and before this it restated every one of those facts by hand: 83 method-name
// literals, dozens of result types, and a parallel union of the nineteen capability
// keys.
//
// The ergonomic SDK still owns transport handles such as AbortSignal, but it reads
// operation and idempotency behavior from the policy emitted here. Otherwise every
// wrapper would have to choose between query and mutation paths by hand, which is
// exactly how a newly added command can accidentally skip replay protection.
const tsMethodsFileName = "wire.methods.generated.ts"

type methodsEmitter struct {
	tsTypes
	out strings.Builder

	// imported collects the published shapes the module names, so its import list is
	// exactly those — the binding compiles with noUnusedLocals.
	imported map[string]bool
}

func newWireMethods(registry *operation.Registry, set *schemaSet) string {
	emitter := &methodsEmitter{tsTypes: tsTypes{set: set}, imported: make(map[string]bool)}
	metas := registry.Metas()

	// Rendered first: the shapes it names decide the import list.
	shapes := emitter.shapes(metas)

	emitter.header()
	emitter.imports()
	emitter.features()
	emitter.names(metas)
	emitter.streamingNames(metas)
	emitter.valueMethodNames(metas)
	emitter.methodPolicy(metas)
	emitter.policy(metas)
	emitter.out.WriteString(shapes)
	emitter.helpers()
	return emitter.out.String()
}

// methodPolicy emits the semantic category and response/replay behavior for every
// method. The SDK consumes this table to attach idempotency keys to commands by
// construction; method wrappers never classify themselves.
func (m *methodsEmitter) methodPolicy(metas []operation.MethodMeta) {
	m.line("export type WireOperationKind = \"query\" | \"command\" | \"subscription\";")
	m.line("export type WireResponseKind = \"unary\" | \"stream\";")
	m.line("export type WireIdempotencyPolicy = \"none\" | \"replayResponse\" | \"replayRunStream\";")
	m.line("export type WireReplayCursorPolicy = \"none\" | \"run\";")
	m.line("export type WirePaginationKind = \"none\" | \"cursor\";")
	m.line("")
	m.line("export interface WireMethodPolicy {")
	m.line("  operation: WireOperationKind;")
	m.line("  response: WireResponseKind;")
	m.line("  idempotency: WireIdempotencyPolicy;")
	m.line("  replayCursor: WireReplayCursorPolicy;")
	m.line("  pagination: WirePaginationKind;")
	m.line("}")
	m.line("")
	m.line("export const WIRE_METHOD_POLICY = {")
	for _, meta := range metas {
		m.line("  %s: {", strconv.Quote(meta.Name.String()))
		m.line("    operation: %s,", strconv.Quote(meta.Operation.String()))
		m.line("    response: %s,", strconv.Quote(meta.Kind.String()))
		m.line("    idempotency: %s,", strconv.Quote(meta.Idempotency.String()))
		m.line("    replayCursor: %s,", strconv.Quote(meta.ReplayCursor.String()))
		m.line("    pagination: %s,", strconv.Quote(meta.Pagination.String()))
		m.line("  },")
	}
	m.line("} as const satisfies { readonly [M in WireMethodName]: WireMethodPolicy };")
	m.line("")
	m.line("/** True only for calls whose first response the runtime durably replays. */")
	m.line("export type WireMutationMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"operation\"] extends \"command\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodRequiresIdempotency(method: WireMethodName): boolean {")
	m.line("  return WIRE_METHOD_POLICY[method].operation === \"command\";")
	m.line("}")
	m.line("")
	m.line("/** Every stream that accepts an opaque Run event replay cursor. */")
	m.line("export type WireReplayCursorMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"replayCursor\"] extends \"run\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodAcceptsReplayCursor(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireReplayCursorMethodName {")
	m.line("  return WIRE_METHOD_POLICY[method].replayCursor === \"run\";")
	m.line("}")
	m.line("")
	m.line("/** Every cursor-paginated method, derived from the method policy above. */")
	m.line("export type WirePaginatedMethodName = {")
	m.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"pagination\"] extends \"cursor\"")
	m.line("    ? M")
	m.line("    : never;")
	m.line("}[WireMethodName];")
	m.line("")
	m.line("export function wireMethodIsPaginated(")
	m.line("  method: WireMethodName,")
	m.line("): method is WirePaginatedMethodName {")
	m.line("  return WIRE_METHOD_POLICY[method].pagination === \"cursor\";")
	m.line("}")
	m.line("")
}

// policy emits the capability rules the SDK preflights against what the server
// advertised. Contract §11.1 names three consumers of these rules — the dispatcher,
// discovery and the SDK — and forbids any of them keeping a second switch. Typing
// the table with WireFeature also makes a rule naming an unpublished key a compile
// error on this side, not only in Go.
func (m *methodsEmitter) policy(metas []operation.MethodMeta) {
	m.line("/** One condition on the request that decides whether a rule applies. */")
	m.line("export interface WireCapabilityCondition {")
	m.line("  field: string;")
	m.line("  operator: \"present\" | \"equals\";")
	m.line("  value?: string;")
	m.line("}")
	m.line("")
	m.line("/**")
	m.line(" * What a method needs negotiated. An absent `when` gates the whole method; a")
	m.line(" * condition gates only the requests that match it.")
	m.line(" */")
	m.line("export interface WireCapabilityRule {")
	m.line("  when?: readonly WireCapabilityCondition[];")
	m.line("  requires: readonly WireFeature[];")
	m.line("}")
	m.line("")
	m.line("export const WIRE_CAPABILITY_POLICY: {")
	m.line("  readonly [M in WireMethodName]?: readonly WireCapabilityRule[];")
	m.line("} = {")
	for _, meta := range metas {
		if len(meta.CapabilityRules) == 0 {
			continue
		}
		m.line("  %s: [", strconv.Quote(meta.Name.String()))
		for _, rule := range meta.CapabilityRules {
			m.line("    { %s },", renderRule(rule))
		}
		m.line("  ],")
	}
	m.line("};")
	m.line("")
}

// renderRule writes one capability rule's members.
func renderRule(rule operation.CapabilityRule) string {
	var members []string
	if len(rule.When) != 0 {
		conditions := make([]string, 0, len(rule.When))
		for _, condition := range rule.When {
			parts := []string{
				"field: " + strconv.Quote(condition.Field),
				"operator: " + strconv.Quote(condition.Operator.String()),
			}
			if condition.Value != "" {
				parts = append(parts, "value: "+strconv.Quote(condition.Value))
			}
			conditions = append(conditions, "{ "+strings.Join(parts, ", ")+" }")
		}
		members = append(members, "when: ["+strings.Join(conditions, ", ")+"]")
	}
	members = append(members, "requires: ["+strings.Join(quoteAll(rule.Requires), ", ")+"]")
	return strings.Join(members, ", ")
}

func (m *methodsEmitter) header() {
	m.line("// Code generated by cmd/contractgen. DO NOT EDIT.")
	m.line("//")
	m.line("// The method surface of the Lyra Runtime Protocol: which methods exist, what each")
	m.line("// one carries, and what has to be negotiated before one will run.")
	m.line("//")
	m.line("// The SDK in methods.ts composes these with transport concerns. Every protocol")
	m.line("// fact in this file is read from the Contract Registry.")
	m.line("")
}

func (m *methodsEmitter) imports() {
	names := slices.Sorted(maps.Keys(m.imported))
	if len(names) == 0 {
		return
	}
	m.line("import type {")
	for _, name := range names {
		m.line("  %s,", name)
	}
	m.line("} from \"./wire.generated\";")
	m.line("")
}

func (m *methodsEmitter) features() {
	m.line("// Every capability key discovery may advertise (API.md §9). Private: it exists to")
	m.line("// derive the union, and a published array with no reader would be a second table.")
	m.line("const FEATURES = [")
	for _, key := range protocol.FeatureKeys() {
		m.line("  %s,", strconv.Quote(key))
	}
	m.line("] as const;")
	m.line("")
	m.line("/** One capability key discovery may advertise. */")
	m.line("export type WireFeature = (typeof FEATURES)[number];")
	m.line("")
}

func (m *methodsEmitter) names(metas []operation.MethodMeta) {
	m.line("// Every method the runtime routes, in registration order.")
	m.line("const METHOD_NAMES = [")
	for _, meta := range metas {
		m.line("  %s,", strconv.Quote(meta.Name.String()))
	}
	m.line("] as const;")
	m.line("")
	m.line("/** One method the runtime routes. */")
	m.line("export type WireMethodName = (typeof METHOD_NAMES)[number];")
	m.line("")
}

func (m *methodsEmitter) streamingNames(metas []operation.MethodMeta) {
	m.line("// Every method whose HTTP response remains open as an event stream.")
	m.line("export const WIRE_STREAMING_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Event != nil {
			m.line("  %s,", strconv.Quote(meta.Name.String()))
		}
	}
	m.line("] as const;")
	m.line("")
	m.line("export type WireStreamingMethodName = (typeof WIRE_STREAMING_METHOD_NAMES)[number];")
	m.line("")
	m.line("export function isWireStreamingMethodName(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireStreamingMethodName {")
	m.line("  return (WIRE_STREAMING_METHOD_NAMES as readonly string[]).includes(method);")
	m.line("}")
	m.line("")
}

func (m *methodsEmitter) valueMethodNames(metas []operation.MethodMeta) {
	m.line("// Methods whose validated wire result becomes a value in the ergonomic SDK.")
	m.line("const VALUE_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Result != nil {
			m.line("  %s,", strconv.Quote(meta.Name.String()))
		}
	}
	m.line("] as const;")
	m.line("")
	m.line("type WireValueMethodName = (typeof VALUE_METHOD_NAMES)[number];")
	m.line("")
	m.line("export function wireMethodReturnsValue(")
	m.line("  method: WireMethodName,")
	m.line("): method is WireValueMethodName {")
	m.line("  return (VALUE_METHOD_NAMES as readonly string[]).includes(method);")
	m.line("}")
	m.line("")
}

// shapes renders the frames each method carries. A method with no result has no
// `result` member rather than carrying `void`: an ack-only method answers with an
// empty success, and WireResult reads that absence.
//
// A stream's events are absent for the same reason its ack is present: the SDK names
// RunEvent and WorkspaceEvent directly, which are already this contract's types, so
// a per-method event alias would only add a second way to spell one of two answers.
func (m *methodsEmitter) shapes(metas []operation.MethodMeta) string {
	var out strings.Builder
	fmt.Fprintln(&out, "/** The frames each method carries. */")
	fmt.Fprintln(&out, "export interface WireShapes {")
	for _, meta := range metas {
		members := []string{"params: " + m.shape(meta.Params)}
		if meta.Result != nil {
			result := m.shape(meta.Result)
			if meta.ResultNullable {
				result += " | null"
			}
			members = append(members, "result: "+result)
		}
		fmt.Fprintf(&out, "  %s: { %s };\n", strconv.Quote(meta.Name.String()), strings.Join(members, "; "))
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out, "")
	return out.String()
}

func (m *methodsEmitter) helpers() {
	m.line("export type WireParams<M extends WireMethodName> = WireShapes[M][\"params\"];")
	m.line("")
	m.line("/** A method with no result answers with an empty success. */")
	m.line("export type WireResult<M extends WireMethodName> =")
	m.line("  WireShapes[M] extends { result: infer R } ? R : void;")
}

// shape names a Go frame type as TypeScript spells it, recording a published shape
// so the import list can name it. A frame with no published definition renders
// inline instead — the empty request struct as `Record<string, never>`, an opaque
// tool result as `unknown` — and there is nothing to import.
func (m *methodsEmitter) shape(t reflect.Type) string {
	name := m.typeName(t)
	if _, published := m.set.defs[name]; published {
		m.imported[name] = true
	}
	return name
}

func (m *methodsEmitter) line(format string, arguments ...any) {
	fmt.Fprintf(&m.out, format+"\n", arguments...)
}
