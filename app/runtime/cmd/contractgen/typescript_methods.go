package main

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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
func (e *methodsEmitter) methodPolicy(metas []operation.MethodMeta) {
	e.line("export type WireOperationKind = \"query\" | \"command\" | \"subscription\";")
	e.line("export type WireResponseKind = \"unary\" | \"stream\";")
	e.line("export type WireIdempotencyPolicy = \"none\" | \"replayResponse\" | \"replayRunStream\";")
	e.line("export type WirePaginationKind = \"none\" | \"cursor\";")
	e.line("")
	e.line("export interface WireMethodPolicy {")
	e.line("  operation: WireOperationKind;")
	e.line("  response: WireResponseKind;")
	e.line("  idempotency: WireIdempotencyPolicy;")
	e.line("  pagination: WirePaginationKind;")
	e.line("}")
	e.line("")
	e.line("export const WIRE_METHOD_POLICY = {")
	for _, meta := range metas {
		e.line("  %s: {", strconv.Quote(meta.Name))
		e.line("    operation: %s,", strconv.Quote(meta.Operation.String()))
		e.line("    response: %s,", strconv.Quote(meta.Kind.String()))
		e.line("    idempotency: %s,", strconv.Quote(meta.Idempotency.String()))
		e.line("    pagination: %s,", strconv.Quote(meta.Pagination.String()))
		e.line("  },")
	}
	e.line("} as const satisfies { readonly [M in WireMethodName]: WireMethodPolicy };")
	e.line("")
	e.line("/** True only for calls whose first response the runtime durably replays. */")
	e.line("export type WireMutationMethodName = {")
	e.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"operation\"] extends \"command\"")
	e.line("    ? M")
	e.line("    : never;")
	e.line("}[WireMethodName];")
	e.line("")
	e.line("export function wireMethodRequiresIdempotency(method: WireMethodName): boolean {")
	e.line("  return WIRE_METHOD_POLICY[method].operation === \"command\";")
	e.line("}")
	e.line("")
	e.line("/** Every cursor-paginated method, derived from the method policy above. */")
	e.line("export type WirePaginatedMethodName = {")
	e.line("  [M in WireMethodName]: (typeof WIRE_METHOD_POLICY)[M][\"pagination\"] extends \"cursor\"")
	e.line("    ? M")
	e.line("    : never;")
	e.line("}[WireMethodName];")
	e.line("")
	e.line("export function wireMethodIsPaginated(")
	e.line("  method: WireMethodName,")
	e.line("): method is WirePaginatedMethodName {")
	e.line("  return WIRE_METHOD_POLICY[method].pagination === \"cursor\";")
	e.line("}")
	e.line("")
}

// policy emits the capability rules the SDK preflights against what the server
// advertised. Contract §11.1 names three consumers of these rules — the dispatcher,
// discovery and the SDK — and forbids any of them keeping a second switch. Typing
// the table with WireFeature also makes a rule naming an unpublished key a compile
// error on this side, not only in Go.
func (e *methodsEmitter) policy(metas []operation.MethodMeta) {
	e.line("/** One condition on the request that decides whether a rule applies. */")
	e.line("export interface WireCapabilityCondition {")
	e.line("  field: string;")
	e.line("  operator: \"present\" | \"equals\";")
	e.line("  value?: string;")
	e.line("}")
	e.line("")
	e.line("/**")
	e.line(" * What a method needs negotiated. An absent `when` gates the whole method; a")
	e.line(" * condition gates only the requests that match it.")
	e.line(" */")
	e.line("export interface WireCapabilityRule {")
	e.line("  when?: readonly WireCapabilityCondition[];")
	e.line("  requires: readonly WireFeature[];")
	e.line("}")
	e.line("")
	e.line("export const WIRE_CAPABILITY_POLICY: {")
	e.line("  readonly [M in WireMethodName]?: readonly WireCapabilityRule[];")
	e.line("} = {")
	for _, meta := range metas {
		if len(meta.CapabilityRules) == 0 {
			continue
		}
		e.line("  %s: [", strconv.Quote(meta.Name))
		for _, rule := range meta.CapabilityRules {
			e.line("    { %s },", renderRule(rule))
		}
		e.line("  ],")
	}
	e.line("};")
	e.line("")
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

func (e *methodsEmitter) header() {
	e.line("// Code generated by cmd/contractgen. DO NOT EDIT.")
	e.line("//")
	e.line("// The method surface of the Lyra Runtime Protocol: which methods exist, what each")
	e.line("// one carries, and what has to be negotiated before one will run.")
	e.line("//")
	e.line("// The SDK in methods.ts composes these with transport concerns. Every protocol")
	e.line("// fact in this file is read from the Contract Registry.")
	e.line("")
}

func (e *methodsEmitter) imports() {
	names := slices.Sorted(maps.Keys(e.imported))
	if len(names) == 0 {
		return
	}
	e.line("import type {")
	for _, name := range names {
		e.line("  %s,", name)
	}
	e.line("} from \"./wire.generated\";")
	e.line("")
}

func (e *methodsEmitter) features() {
	e.line("// Every capability key discovery may advertise (API.md §9). Private: it exists to")
	e.line("// derive the union, and a published array with no reader would be a second table.")
	e.line("const FEATURES = [")
	for _, key := range protocol.FeatureKeys() {
		e.line("  %s,", strconv.Quote(key))
	}
	e.line("] as const;")
	e.line("")
	e.line("/** One capability key discovery may advertise. */")
	e.line("export type WireFeature = (typeof FEATURES)[number];")
	e.line("")
}

func (e *methodsEmitter) names(metas []operation.MethodMeta) {
	e.line("// Every method the runtime routes, in registration order.")
	e.line("const METHOD_NAMES = [")
	for _, meta := range metas {
		e.line("  %s,", strconv.Quote(meta.Name))
	}
	e.line("] as const;")
	e.line("")
	e.line("/** One method the runtime routes. */")
	e.line("export type WireMethodName = (typeof METHOD_NAMES)[number];")
	e.line("")
}

func (e *methodsEmitter) streamingNames(metas []operation.MethodMeta) {
	e.line("// Every method whose HTTP response remains open as an event stream.")
	e.line("export const WIRE_STREAMING_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Event != nil {
			e.line("  %s,", strconv.Quote(meta.Name))
		}
	}
	e.line("] as const;")
	e.line("")
	e.line("export type WireStreamingMethodName = (typeof WIRE_STREAMING_METHOD_NAMES)[number];")
	e.line("")
	e.line("export function isWireStreamingMethodName(")
	e.line("  method: WireMethodName,")
	e.line("): method is WireStreamingMethodName {")
	e.line("  return (WIRE_STREAMING_METHOD_NAMES as readonly string[]).includes(method);")
	e.line("}")
	e.line("")
}

func (e *methodsEmitter) valueMethodNames(metas []operation.MethodMeta) {
	e.line("// Methods whose validated wire result becomes a value in the ergonomic SDK.")
	e.line("const VALUE_METHOD_NAMES = [")
	for _, meta := range metas {
		if meta.Result != nil {
			e.line("  %s,", strconv.Quote(meta.Name))
		}
	}
	e.line("] as const;")
	e.line("")
	e.line("type WireValueMethodName = (typeof VALUE_METHOD_NAMES)[number];")
	e.line("")
	e.line("export function wireMethodReturnsValue(")
	e.line("  method: WireMethodName,")
	e.line("): method is WireValueMethodName {")
	e.line("  return (VALUE_METHOD_NAMES as readonly string[]).includes(method);")
	e.line("}")
	e.line("")
}

// shapes renders the frames each method carries. A method with no result has no
// `result` member rather than carrying `void`: an ack-only method answers with an
// empty success, and WireResult reads that absence.
//
// A stream's events are absent for the same reason its ack is present: the SDK names
// RunEvent and WorkspaceEvent directly, which are already this contract's types, so
// a per-method event alias would only add a second way to spell one of two answers.
func (e *methodsEmitter) shapes(metas []operation.MethodMeta) string {
	var out strings.Builder
	fmt.Fprintln(&out, "/** The frames each method carries. */")
	fmt.Fprintln(&out, "export interface WireShapes {")
	for _, meta := range metas {
		members := []string{"params: " + e.shape(meta.Params)}
		if meta.Result != nil {
			result := e.shape(meta.Result)
			if meta.ResultNullable {
				result += " | null"
			}
			members = append(members, "result: "+result)
		}
		fmt.Fprintf(&out, "  %s: { %s };\n", strconv.Quote(meta.Name), strings.Join(members, "; "))
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out, "")
	return out.String()
}

func (e *methodsEmitter) helpers() {
	e.line("export type WireParams<M extends WireMethodName> = WireShapes[M][\"params\"];")
	e.line("")
	e.line("/** A method with no result answers with an empty success. */")
	e.line("export type WireResult<M extends WireMethodName> =")
	e.line("  WireShapes[M] extends { result: infer R } ? R : void;")
}

// shape names a Go frame type as TypeScript spells it, recording a published shape
// so the import list can name it. A frame with no published definition renders
// inline instead — the empty request struct as `Record<string, never>`, an opaque
// tool result as `unknown` — and there is nothing to import.
func (e *methodsEmitter) shape(t reflect.Type) string {
	name := e.typeName(t)
	if _, published := e.set.defs[name]; published {
		e.imported[name] = true
	}
	return name
}

func (e *methodsEmitter) line(format string, arguments ...any) {
	fmt.Fprintf(&e.out, format+"\n", arguments...)
}
