// Package contractgen projects the canonical Go contract into checked-in artifacts.
package contractgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Manifest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Methods         []MethodManifest   `json:"methods"`
	Endpoints       []EndpointManifest `json:"endpoints"`
	Enums           []EnumManifest     `json:"enums"`
	Features        []protocol.Feature `json:"features"`
}

type MethodManifest struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Operation      string   `json:"operation"`
	Idempotency    string   `json:"idempotency"`
	Pagination     string   `json:"pagination"`
	Params         string   `json:"params"`
	Result         string   `json:"result"`
	ResultNullable bool     `json:"resultNullable"`
	Event          string   `json:"event,omitempty"`
	Errors         []string `json:"errors"`
	Materializes   []string `json:"materializes"`
}

type EndpointManifest struct {
	Name             string `json:"name"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	Authentication   string `json:"authentication"`
	ResponseStatuses []int  `json:"responseStatuses"`
	ResponseType     string `json:"responseType,omitempty"`
}

type EnumManifest struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type enumSpec struct {
	typeOf reflect.Type
	values []string
}

func Artifacts() (map[string][]byte, error) {
	manifest := buildManifest()
	var manifestBuffer bytes.Buffer
	encoder := json.NewEncoder(&manifestBuffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return nil, fmt.Errorf("contractgen: encode manifest: %w", err)
	}
	encodedManifest := manifestBuffer.Bytes()
	wire, err := renderWire(manifest)
	if err != nil {
		return nil, err
	}
	client := renderClient(manifest)
	return map[string][]byte{
		"manifest.json":                  encodedManifest,
		"typescript/wire.generated.ts":   wire,
		"typescript/client.generated.ts": client,
		"typescript/index.ts":            []byte("export * from \"./wire.generated.js\";\nexport * from \"./client.generated.js\";\n"),
	}, nil
}

func buildManifest() Manifest {
	methods := operation.Contract().Metas()
	sort.Slice(methods, func(left, right int) bool { return methods[left].Name < methods[right].Name })
	methodManifest := make([]MethodManifest, 0, len(methods))
	for _, method := range methods {
		event := ""
		if method.Event != nil {
			event = typeName(method.Event)
		}
		methodManifest = append(methodManifest, MethodManifest{
			Name: method.Name, Kind: method.Kind.String(), Operation: method.Operation.String(),
			Idempotency: method.Idempotency.String(), Pagination: method.Pagination.String(),
			Params: typeName(method.Params), Result: typeName(method.Result), ResultNullable: method.ResultNullable, Event: event,
			Errors: nonNil(method.Errors), Materializes: nonNil(method.Materializes),
		})
	}
	endpoints := httptransport.Contract()
	endpointManifest := make([]EndpointManifest, 0, len(endpoints))
	for _, endpoint := range endpoints {
		responseType := ""
		if endpoint.ResponseType != nil {
			responseType = typeName(endpoint.ResponseType)
		}
		endpointManifest = append(endpointManifest, EndpointManifest{
			Name: endpoint.Name, Method: endpoint.Method, Path: endpoint.Path,
			Authentication:   string(endpoint.Authentication),
			ResponseStatuses: slices.Clone(endpoint.ResponseStatuses), ResponseType: responseType,
		})
	}
	enums := enumSpecs()
	enumManifest := make([]EnumManifest, 0, len(enums))
	for _, enum := range enums {
		enumManifest = append(enumManifest, EnumManifest{Name: enum.typeOf.Name(), Values: slices.Clone(enum.values)})
	}
	sort.Slice(enumManifest, func(left, right int) bool { return enumManifest[left].Name < enumManifest[right].Name })
	return Manifest{
		ProtocolVersion: protocol.ProtocolVersion,
		Methods:         methodManifest, Endpoints: endpointManifest, Enums: enumManifest,
		Features: protocol.Features(),
	}
}

func enumSpecs() []enumSpec {
	return []enumSpec{
		{reflect.TypeFor[protocol.InterruptType](), stringsOf(protocol.InterruptTypes())},
		{reflect.TypeFor[protocol.SuppressibleRunEventType](), stringsOf(protocol.SuppressibleRunEventTypes())},
		{reflect.TypeFor[protocol.StreamEventType](), stringsOf(protocol.RunEventTypes())},
		{reflect.TypeFor[protocol.RuntimeTopic](), stringsOf(protocol.RuntimeTopics())},
		{reflect.TypeFor[protocol.RunReplayScope](), stringsOf(protocol.RunReplayScopes())},
		{reflect.TypeFor[httptransport.HealthStatus](), []string{string(httptransport.HealthOK), string(httptransport.HealthDegraded), string(httptransport.HealthUnhealthy)}},
		{reflect.TypeFor[httptransport.TransportKind](), []string{string(httptransport.TransportHTTP)}},
	}
}

func stringsOf[T ~string](values []T) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = string(value)
	}
	return out
}

func nonNil[T any](values []T) []T {
	if len(values) == 0 {
		return []T{}
	}
	return slices.Clone(values)
}

func typeName(typeOf reflect.Type) string {
	if typeOf == nil {
		return "EmptyObject"
	}
	if typeOf == reflect.TypeFor[struct{}]() {
		return "EmptyObject"
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if element, page := pageElement(typeOf); page {
		return "Page<" + typeName(element) + ">"
	}
	if typeOf.Name() != "" {
		return typeOf.Name()
	}
	return typeOf.String()
}

type typeRegistry struct {
	definitions map[string]reflect.Type
	enums       map[reflect.Type][]string
}

func newTypeRegistry() *typeRegistry {
	registry := &typeRegistry{definitions: make(map[string]reflect.Type), enums: make(map[reflect.Type][]string)}
	for _, enum := range enumSpecs() {
		registry.enums[enum.typeOf] = enum.values
	}
	return registry
}

func (registry *typeRegistry) add(typeOf reflect.Type) {
	if typeOf == nil {
		return
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf == reflect.TypeFor[struct{}]() {
		return
	}
	if typeOf == reflect.TypeFor[time.Time]() || typeOf == reflect.TypeFor[json.RawMessage]() {
		return
	}
	if element, page := pageElement(typeOf); page {
		registry.add(element)
		return
	}
	if isContractDefinition(typeOf) {
		if existing, found := registry.definitions[typeOf.Name()]; found {
			if existing != typeOf {
				panic("contractgen: colliding Go type names: " + typeOf.Name())
			}
			return
		}
		registry.definitions[typeOf.Name()] = typeOf
	}
	switch typeOf.Kind() {
	case reflect.Struct:
		for _, field := range contractFields(typeOf) {
			registry.add(field.typeOf)
		}
	case reflect.Slice, reflect.Array, reflect.Pointer:
		registry.add(typeOf.Elem())
	case reflect.Map:
		registry.add(typeOf.Elem())
	}
}

func renderWire(manifest Manifest) ([]byte, error) {
	registry := newTypeRegistry()
	registry.add(reflect.TypeFor[protocol.RequestMeta]())
	registry.add(reflect.TypeFor[protocol.ProblemData]())
	for _, method := range operation.Contract().Metas() {
		registry.add(method.Params)
		registry.add(method.Result)
		registry.add(method.Event)
	}
	for _, endpoint := range httptransport.Contract() {
		registry.add(endpoint.ResponseType)
	}

	var output strings.Builder
	output.WriteString("/* Code generated by contractgen. DO NOT EDIT. */\n\n")
	fmt.Fprintf(&output, "export const protocolVersion = %s as const;\n\n", strconv.Quote(manifest.ProtocolVersion))
	output.WriteString("export type EmptyObject = Record<string, never>;\n\n")
	output.WriteString("export interface Page<Value> {\n  data: Array<Value>;\n  nextCursor?: string;\n}\n\n")

	names := make([]string, 0, len(registry.definitions))
	for name := range registry.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		typeOf := registry.definitions[name]
		if values, enum := registry.enums[typeOf]; enum {
			fmt.Fprintf(&output, "export type %s = %s;\n\n", name, union(values))
			continue
		}
		if typeOf.Kind() == reflect.String {
			fmt.Fprintf(&output, "export type %s = string;\n\n", name)
			continue
		}
		fields := contractFields(typeOf)
		if len(fields) == 0 {
			fmt.Fprintf(&output, "export type %s = EmptyObject;\n\n", name)
			continue
		}
		fmt.Fprintf(&output, "export interface %s {\n", name)
		for _, field := range fields {
			fmt.Fprintf(&output, "  %s%s: %s;\n", propertyName(field.name), optionalMark(field.optional), registry.tsType(field.typeOf))
		}
		output.WriteString("}\n\n")
	}

	methods := operation.Contract().Metas()
	output.WriteString("export interface UnaryRuntimeMethods {\n")
	for _, method := range methods {
		if method.Kind != operation.KindUnary {
			continue
		}
		fmt.Fprintf(&output, "  %s: { params: %s; result: %s };\n", strconv.Quote(method.Name), registry.tsType(method.Params), registry.methodResultType(method))
	}
	output.WriteString("}\n\n")
	output.WriteString("export interface StreamRuntimeMethods {\n")
	for _, method := range methods {
		if method.Kind != operation.KindStream {
			continue
		}
		fmt.Fprintf(&output, "  %s: { params: %s; result: %s; event: %s };\n", strconv.Quote(method.Name), registry.tsType(method.Params), registry.methodResultType(method), registry.tsType(method.Event))
	}
	output.WriteString("}\n\n")
	output.WriteString("export type RuntimeMethods = UnaryRuntimeMethods & StreamRuntimeMethods;\n\n")
	output.WriteString("export const runtimeMethodFacts = {\n")
	for _, method := range methods {
		fmt.Fprintf(&output, "  %s: { kind: %s, operation: %s, idempotency: %s },\n", strconv.Quote(method.Name), strconv.Quote(method.Kind.String()), strconv.Quote(method.Operation.String()), strconv.Quote(method.Idempotency.String()))
	}
	output.WriteString("} as const satisfies { [Name in keyof RuntimeMethods]: { kind: \"unary\" | \"stream\"; operation: \"query\" | \"command\" | \"subscription\"; idempotency: \"none\" | \"replayResponse\" | \"replayRunStream\" } };\n\n")
	output.WriteString("function isRecord(value: unknown): value is Record<string, unknown> {\n  return typeof value === \"object\" && value !== null && !Array.isArray(value);\n}\n\n")
	output.WriteString("function isEmptyObject(value: unknown): value is EmptyObject {\n  return isRecord(value) && Object.keys(value).length === 0;\n}\n\n")
	output.WriteString("function isTimestamp(value: unknown): value is string {\n  return typeof value === \"string\" && /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d{1,9})?(?:Z|[+-]\\d{2}:\\d{2})$/.test(value) && !Number.isNaN(Date.parse(value));\n}\n\n")
	output.WriteString("function isPage<Value>(value: unknown, isValue: (value: unknown) => value is Value): value is Page<Value> {\n  return isRecord(value) && hasOnlyKeys(value, [\"data\"], [\"nextCursor\"]) && Array.isArray(value.data) && value.data.every(isValue) && (!(\"nextCursor\" in value) || typeof value.nextCursor === \"string\");\n}\n\n")
	output.WriteString("function hasOnlyKeys(value: Record<string, unknown>, required: readonly string[], optional: readonly string[]): boolean {\n  const allowed = new Set([...required, ...optional]);\n  return required.every((key) => key in value) && Object.keys(value).every((key) => allowed.has(key));\n}\n\n")
	for _, name := range names {
		typeOf := registry.definitions[name]
		fmt.Fprintf(&output, "export function is%s(value: unknown): value is %s {\n", name, name)
		if values, enum := registry.enums[typeOf]; enum {
			fmt.Fprintf(&output, "  return typeof value === \"string\" && [%s].includes(value);\n", quotedList(values))
		} else if typeOf.Kind() == reflect.String {
			output.WriteString("  return typeof value === \"string\";\n")
		} else {
			output.WriteString("  if (!isRecord(value)) return false;\n")
			fields := contractFields(typeOf)
			required, optional := contractFieldNames(fields)
			fmt.Fprintf(&output, "  if (!hasOnlyKeys(value, [%s], [%s])) return false;\n", quotedList(required), quotedList(optional))
			for _, field := range fields {
				expression := registry.validator(field.typeOf, "value["+strconv.Quote(field.name)+"]")
				if field.optional {
					fmt.Fprintf(&output, "  if (%s in value && !(%s)) return false;\n", strconv.Quote(field.name), expression)
				} else {
					fmt.Fprintf(&output, "  if (!(%s)) return false;\n", expression)
				}
			}
			output.WriteString("  return true;\n")
		}
		output.WriteString("}\n\n")
	}
	output.WriteString("export const unaryResultValidators: {\n  [Name in keyof UnaryRuntimeMethods]: (value: unknown) => value is UnaryRuntimeMethods[Name][\"result\"];\n} = {\n")
	for _, method := range methods {
		if method.Kind != operation.KindUnary {
			continue
		}
		fmt.Fprintf(&output, "  %s: (value: unknown): value is %s => %s,\n", strconv.Quote(method.Name), registry.methodResultType(method), registry.methodResultValidator(method, "value"))
	}
	output.WriteString("};\n\n")
	output.WriteString("export const streamResultValidators: {\n  [Name in keyof StreamRuntimeMethods]: (value: unknown) => value is StreamRuntimeMethods[Name][\"result\"];\n} = {\n")
	for _, method := range methods {
		if method.Kind != operation.KindStream {
			continue
		}
		fmt.Fprintf(&output, "  %s: (value: unknown): value is %s => %s,\n", strconv.Quote(method.Name), registry.methodResultType(method), registry.methodResultValidator(method, "value"))
	}
	output.WriteString("};\n\n")
	output.WriteString("export const streamEventValidators: {\n  [Name in keyof StreamRuntimeMethods]: (value: unknown) => value is StreamRuntimeMethods[Name][\"event\"];\n} = {\n")
	for _, method := range methods {
		if method.Kind != operation.KindStream {
			continue
		}
		fmt.Fprintf(&output, "  %s: (value: unknown): value is %s => %s,\n", strconv.Quote(method.Name), registry.tsType(method.Event), registry.validator(method.Event, "value"))
	}
	output.WriteString("};\n\n")
	output.WriteString("export function assertDiscoverResponse(value: unknown): asserts value is DiscoverResponse {\n  if (!isDiscoverResponse(value)) throw new TypeError(\"invalid runtime.discover response\");\n}\n")
	return []byte(output.String()), nil
}

func (registry *typeRegistry) tsType(typeOf reflect.Type) string {
	if typeOf == nil {
		return "EmptyObject"
	}
	if typeOf == reflect.TypeFor[struct{}]() {
		return "EmptyObject"
	}
	if typeOf == reflect.TypeFor[time.Time]() {
		return "string"
	}
	if typeOf == reflect.TypeFor[json.RawMessage]() {
		return "unknown"
	}
	if typeOf.Kind() == reflect.Pointer {
		return registry.tsType(typeOf.Elem()) + " | null"
	}
	if element, page := pageElement(typeOf); page {
		return "Page<" + registry.tsType(element) + ">"
	}
	if isContractDefinition(typeOf) {
		return typeOf.Name()
	}
	switch typeOf.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "Array<" + registry.tsType(typeOf.Elem()) + ">"
	case reflect.Map:
		return "Record<string, " + registry.tsType(typeOf.Elem()) + ">"
	case reflect.Interface:
		return "unknown"
	default:
		panic("contractgen: unsupported TypeScript type " + typeOf.String())
	}
}

func (registry *typeRegistry) methodResultType(method operation.MethodMeta) string {
	result := method.Result
	if result == nil {
		return "EmptyObject"
	}
	if result.Kind() == reflect.Pointer {
		result = result.Elem()
	}
	rendered := registry.tsType(result)
	if method.ResultNullable {
		return rendered + " | null"
	}
	return rendered
}

func (registry *typeRegistry) methodResultValidator(method operation.MethodMeta, expression string) string {
	result := method.Result
	if result == nil {
		return registry.validator(nil, expression)
	}
	if result.Kind() == reflect.Pointer {
		result = result.Elem()
	}
	validated := registry.validator(result, expression)
	if method.ResultNullable {
		return expression + " === null || " + validated
	}
	return validated
}

func (registry *typeRegistry) validator(typeOf reflect.Type, expression string) string {
	if typeOf == nil || typeOf == reflect.TypeFor[struct{}]() {
		return "isEmptyObject(" + expression + ")"
	}
	if typeOf == reflect.TypeFor[time.Time]() {
		return "isTimestamp(" + expression + ")"
	}
	if typeOf == reflect.TypeFor[json.RawMessage]() {
		return "true"
	}
	if typeOf.Kind() == reflect.Pointer {
		return expression + " === null || " + registry.validator(typeOf.Elem(), expression)
	}
	if element, page := pageElement(typeOf); page {
		return "isPage(" + expression + ", (entry: unknown): entry is " + registry.tsType(element) + " => " + registry.validator(element, "entry") + ")"
	}
	if isContractDefinition(typeOf) {
		return "is" + typeOf.Name() + "(" + expression + ")"
	}
	switch typeOf.Kind() {
	case reflect.String:
		return "typeof " + expression + " === \"string\""
	case reflect.Bool:
		return "typeof " + expression + " === \"boolean\""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Number.isInteger(" + expression + ")"
	case reflect.Float32, reflect.Float64:
		return "typeof " + expression + " === \"number\" && Number.isFinite(" + expression + ")"
	case reflect.Slice, reflect.Array:
		return "Array.isArray(" + expression + ") && " + expression + ".every((entry) => " + registry.validator(typeOf.Elem(), "entry") + ")"
	case reflect.Map:
		return "isRecord(" + expression + ") && Object.values(" + expression + ").every((entry) => " + registry.validator(typeOf.Elem(), "entry") + ")"
	case reflect.Interface:
		return "true"
	default:
		panic("contractgen: unsupported validator type " + typeOf.String())
	}
}

func renderClient(manifest Manifest) []byte {
	var output strings.Builder
	output.WriteString(`/* Code generated by contractgen. DO NOT EDIT. */

import {
  assertDiscoverResponse,
  protocolVersion,
  runtimeMethodFacts,
  streamEventValidators,
  streamResultValidators,
  unaryResultValidators,
  type DiscoverResponse,
  type RequestMeta,
  type RuntimeMethods,
  type StreamRuntimeMethods,
  type UnaryRuntimeMethods,
} from "./wire.generated.js";

export interface RuntimeConnection {
  endpoint: string;
  bearerToken: string;
  instanceId: string;
  protocolVersion: typeof protocolVersion;
  idempotencyNamespace: string;
  generation: number;
}

export interface RuntimeCallOptions {
  meta?: RequestMeta;
  signal?: AbortSignal;
  idempotencyKey?: string;
}

export interface RuntimeStreamOptions extends RuntimeCallOptions {
  afterEventId?: string;
}

export interface RuntimeStreamFrame<Event> {
  event: Event;
  eventId?: string;
}

export interface OpenRuntimeStream<Acknowledgement, Event> extends AsyncIterable<RuntimeStreamFrame<Event>> {
  readonly acknowledgement: Acknowledgement;
  readonly idempotencyKey?: string;
  close(reason?: unknown): void;
}

export class LyraRPCError extends Error {
  readonly code: number;
  readonly data: unknown;

  constructor(code: number, message: string, data: unknown) {
    super(message);
    this.name = "LyraRPCError";
    this.code = code;
    this.data = data;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, allowed: readonly string[]): boolean {
  const keys = Object.keys(value);
  return keys.length === allowed.length && keys.every((key) => allowed.includes(key));
}

function decodeRPCResult<Result>(
  envelope: unknown,
  id: string,
  validator: (value: unknown) => value is Result,
  method: string,
): Result {
  if (!isRecord(envelope) || envelope.jsonrpc !== "2.0" || envelope.id !== id) {
    throw new TypeError("Runtime returned an invalid JSON-RPC envelope");
  }
  const hasResult = "result" in envelope;
  const hasError = "error" in envelope;
  if (hasResult === hasError || !hasOnlyKeys(envelope, hasResult ? ["jsonrpc", "id", "result"] : ["jsonrpc", "id", "error"])) {
    throw new TypeError("Runtime returned an ambiguous JSON-RPC envelope");
  }
  if (hasError) {
    const failure = envelope.error;
    if (
      !isRecord(failure) ||
      typeof failure.code !== "number" ||
      !Number.isInteger(failure.code) ||
      typeof failure.message !== "string" ||
      Object.keys(failure).some((key) => !["code", "message", "data"].includes(key))
    ) {
      throw new TypeError("Runtime returned an invalid JSON-RPC error");
    }
    throw new LyraRPCError(failure.code, failure.message, failure.data);
  }
  if (!validator(envelope.result)) {
    throw new TypeError("Runtime returned an invalid " + method + " result");
  }
  return envelope.result;
}

interface RequestHeaders {
  headers: Record<string, string>;
  idempotencyKey?: string;
}

function requestHeaders(
  connection: RuntimeConnection,
  method: keyof RuntimeMethods,
  options: RuntimeCallOptions,
  accept: "application/json" | "text/event-stream",
): RequestHeaders {
  const facts = runtimeMethodFacts[method];
  if (facts.idempotency === "none" && options.idempotencyKey !== undefined) {
    throw new TypeError(String(method) + " does not accept an idempotency key");
  }
  const idempotencyKey = facts.idempotency === "none" ? undefined : options.idempotencyKey ?? crypto.randomUUID();
  if (idempotencyKey !== undefined && idempotencyKey.trim() === "") {
    throw new TypeError("Idempotency key must not be empty");
  }
  const headers: Record<string, string> = {
    Accept: accept,
    Authorization: "Bearer " + connection.bearerToken,
    "Content-Type": "application/json",
  };
  if (idempotencyKey !== undefined) {
    headers["Idempotency-Key"] = idempotencyKey;
    headers["Idempotency-Namespace"] = connection.idempotencyNamespace;
  }
  return { headers, ...(idempotencyKey === undefined ? {} : { idempotencyKey }) };
}

function requestParams(params: object, meta: RequestMeta | undefined): object {
  return meta === undefined || Object.keys(meta).length === 0 ? params : { ...params, _meta: meta };
}

async function transportFailure(response: Response): Promise<Error> {
  const contentType = response.headers.get("Content-Type") ?? "";
  if (contentType.startsWith("application/problem+json")) {
    const problem: unknown = await response.json().catch(() => undefined);
    if (isRecord(problem) && typeof problem.detail === "string") {
      return new Error("Runtime transport failed with HTTP " + response.status + ": " + problem.detail);
    }
  }
  return new Error("Runtime transport failed with HTTP " + response.status);
}

interface SSEFrame {
  data: string;
  id?: string;
}

async function* readSSEFrames(body: ReadableStream<Uint8Array>): AsyncGenerator<SSEFrame> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let data: string[] = [];
  let id: string | undefined;
  try {
    while (true) {
      const chunk = await reader.read();
      buffer += decoder.decode(chunk.value, { stream: !chunk.done });
      let newline = buffer.indexOf("\n");
      while (newline >= 0) {
        let line = buffer.slice(0, newline);
        buffer = buffer.slice(newline + 1);
        if (line.endsWith("\r")) line = line.slice(0, -1);
        if (line === "") {
          if (data.length > 0) {
            yield { data: data.join("\n"), ...(id === undefined ? {} : { id }) };
          }
          data = [];
          id = undefined;
        } else if (!line.startsWith(":")) {
          const colon = line.indexOf(":");
          const field = colon < 0 ? line : line.slice(0, colon);
          const rawValue = colon < 0 ? "" : line.slice(colon + 1);
          const value = rawValue.startsWith(" ") ? rawValue.slice(1) : rawValue;
          if (field === "data") data.push(value);
          if (field === "id" && !value.includes("\0")) id = value;
        }
        newline = buffer.indexOf("\n");
      }
      if (chunk.done) break;
    }
    if (data.length > 0) {
      yield { data: data.join("\n"), ...(id === undefined ? {} : { id }) };
    }
  } finally {
    reader.releaseLock();
  }
}

function relayAbort(source: AbortSignal | undefined, target: AbortController): () => void {
  if (source === undefined) return () => undefined;
  const forward = () => target.abort(source.reason);
  if (source.aborted) {
    forward();
    return () => undefined;
  }
  source.addEventListener("abort", forward, { once: true });
  return () => source.removeEventListener("abort", forward);
}

function streamNotificationMethod(method: keyof StreamRuntimeMethods): string {
  switch (method) {
`)
	for _, method := range operation.Contract().Metas() {
		if method.Kind != operation.KindStream {
			continue
		}
		notification, _ := streamTransportBinding(method.Event)
		fmt.Fprintf(&output, "    case %s: return %s;\n", strconv.Quote(method.Name), strconv.Quote(notification))
	}
	output.WriteString(`  }
}

function selectStreamEvent(method: keyof StreamRuntimeMethods, params: unknown): unknown {
  switch (method) {
`)
	for _, method := range operation.Contract().Metas() {
		if method.Kind != operation.KindStream {
			continue
		}
		_, wrapped := streamTransportBinding(method.Event)
		if wrapped {
			fmt.Fprintf(&output, "    case %s: return isRecord(params) ? params[\"event\"] : undefined;\n", strconv.Quote(method.Name))
		} else {
			fmt.Fprintf(&output, "    case %s: return params;\n", strconv.Quote(method.Name))
		}
	}
	output.WriteString(`  }
}

function decodeStreamEvent<Name extends keyof StreamRuntimeMethods>(
  method: Name,
  envelope: unknown,
): StreamRuntimeMethods[Name]["event"] {
  if (
    !isRecord(envelope) ||
    envelope.jsonrpc !== "2.0" ||
    envelope.method !== streamNotificationMethod(method) ||
    !hasOnlyKeys(envelope, ["jsonrpc", "method", "params"])
  ) {
    throw new TypeError("Runtime returned an invalid " + String(method) + " notification");
  }
  const candidate = selectStreamEvent(method, envelope.params);
  const validator: (value: unknown) => boolean = streamEventValidators[method];
  if (!validator(candidate)) {
    throw new TypeError("Runtime returned an invalid " + String(method) + " event");
  }
  return candidate as StreamRuntimeMethods[Name]["event"];
}

export class LyraClient {
  readonly #connection: RuntimeConnection;
  readonly #rpcURL: URL;

  constructor(connection: RuntimeConnection) {
    if (connection.protocolVersion !== protocolVersion) {
      throw new TypeError("Unsupported Runtime protocol " + connection.protocolVersion);
    }
    const endpoint = new URL(connection.endpoint);
	const local = endpoint.protocol === "http:" && ["127.0.0.1", "[::1]"].includes(endpoint.hostname);
	const remote = endpoint.protocol === "https:" && endpoint.hostname !== "";
	if ((!local && !remote) || endpoint.username !== "" || endpoint.password !== "") {
	  throw new TypeError("Runtime endpoint must be loopback HTTP or authenticated HTTPS");
    }
	if (endpoint.pathname !== "/" || endpoint.search !== "" || endpoint.hash !== "") {
	  throw new TypeError("Runtime endpoint must be an origin-only URL");
    }
    this.#connection = { ...connection };
    this.#rpcURL = new URL("/v2/rpc", endpoint);
  }

  async discover(meta: RequestMeta = {}, signal?: AbortSignal): Promise<DiscoverResponse> {
    const response = await this.call("runtime.discover", {}, { meta, signal });
    assertDiscoverResponse(response);
    return response;
  }

  async call<Name extends keyof UnaryRuntimeMethods>(
    method: Name,
    params: UnaryRuntimeMethods[Name]["params"],
    options: RuntimeCallOptions = {},
  ): Promise<UnaryRuntimeMethods[Name]["result"]> {
    const id = crypto.randomUUID();
    const request = requestHeaders(this.#connection, method, options, "application/json");
    const response = await fetch(this.#rpcURL, {
      method: "POST",
      headers: request.headers,
      body: JSON.stringify({ jsonrpc: "2.0", id, method, params: requestParams(params, options.meta) }),
      signal: options.signal,
    });
    if (!response.ok || !response.headers.get("Content-Type")?.startsWith("application/json")) {
      throw await transportFailure(response);
    }
    const envelope: unknown = await response.json();
    return decodeRPCResult(envelope, id, unaryResultValidators[method], String(method));
  }

  async stream<Name extends keyof StreamRuntimeMethods>(
    method: Name,
    params: StreamRuntimeMethods[Name]["params"],
    options: RuntimeStreamOptions = {},
  ): Promise<OpenRuntimeStream<StreamRuntimeMethods[Name]["result"], StreamRuntimeMethods[Name]["event"]>> {
    const id = crypto.randomUUID();
    const controller = new AbortController();
    const detachAbort = relayAbort(options.signal, controller);
    let handedOff = false;
    try {
      const request = requestHeaders(this.#connection, method, options, "text/event-stream");
      if (options.afterEventId !== undefined) request.headers["Last-Event-Id"] = options.afterEventId;
      const response = await fetch(this.#rpcURL, {
        method: "POST",
        headers: request.headers,
        body: JSON.stringify({ jsonrpc: "2.0", id, method, params: requestParams(params, options.meta) }),
        signal: controller.signal,
      });
      if (!response.ok) throw await transportFailure(response);
      const contentType = response.headers.get("Content-Type") ?? "";
      if (contentType.startsWith("application/json")) {
        const envelope: unknown = await response.json();
        decodeRPCResult(envelope, id, streamResultValidators[method], String(method));
        throw new TypeError("Runtime returned a unary response for streaming method " + String(method));
      }
      if (!contentType.startsWith("text/event-stream") || response.body === null) {
        throw new TypeError("Runtime returned an invalid streaming transport for " + String(method));
      }

      const frames = readSSEFrames(response.body)[Symbol.asyncIterator]();
      const first = await frames.next();
      if (first.done) throw new TypeError("Runtime closed " + String(method) + " before its acknowledgement");
      const acknowledgementEnvelope: unknown = JSON.parse(first.value.data);
      const acknowledgement = decodeRPCResult(
        acknowledgementEnvelope,
        id,
        streamResultValidators[method],
        String(method),
      );
      const events = (async function* (): AsyncGenerator<RuntimeStreamFrame<StreamRuntimeMethods[Name]["event"]>> {
        try {
          while (true) {
            const next = await frames.next();
            if (next.done) return;
            const envelope: unknown = JSON.parse(next.value.data);
            const event = decodeStreamEvent(method, envelope);
            yield { event, ...(next.value.id === undefined ? {} : { eventId: next.value.id }) };
          }
        } finally {
          await frames.return?.(undefined);
          detachAbort();
          controller.abort();
        }
      })();
      const close = (reason?: unknown) => {
        detachAbort();
        controller.abort(reason);
        void frames.return?.(undefined);
      };
      handedOff = true;
      return {
        acknowledgement,
        ...(request.idempotencyKey === undefined ? {} : { idempotencyKey: request.idempotencyKey }),
        close,
        [Symbol.asyncIterator]: () => events,
      };
    } finally {
      if (!handedOff) {
        detachAbort();
        controller.abort();
      }
    }
  }
}
`)
	_ = manifest
	return []byte(output.String())
}

func streamTransportBinding(event reflect.Type) (notification string, wrapped bool) {
	switch event {
	case reflect.TypeFor[protocol.RunEvent]():
		return "notifications.run.event", false
	case reflect.TypeFor[protocol.RuntimeEvent]():
		return "notifications.runtime.event", true
	default:
		panic("contractgen: HTTP stream binding is missing for " + event.String())
	}
}

func union(values []string) string {
	if len(values) == 0 {
		return "never"
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = strconv.Quote(value)
	}
	return strings.Join(quoted, " | ")
}

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = strconv.Quote(value)
	}
	return strings.Join(quoted, ", ")
}

func jsonField(field reflect.StructField) (string, bool) {
	name, options := jsonNameAndOptions(field)
	return name, slices.Contains(options, "omitempty") || slices.Contains(options, "omitzero")
}

func jsonName(field reflect.StructField) string {
	name, _ := jsonNameAndOptions(field)
	return name
}

func jsonNameAndOptions(field reflect.StructField) (string, []string) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "-", nil
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	return name, parts[1:]
}

type contractField struct {
	name     string
	typeOf   reflect.Type
	optional bool
}

func contractFields(typeOf reflect.Type) []contractField {
	fields := make([]contractField, 0, typeOf.NumField())
	seen := make(map[string]struct{})
	var add func(reflect.Type, bool)
	add = func(structType reflect.Type, inheritedOptional bool) {
		for index := range structType.NumField() {
			field := structType.Field(index)
			if !field.IsExported() || jsonName(field) == "-" {
				continue
			}
			fieldType := field.Type
			underlying := fieldType
			if underlying.Kind() == reflect.Pointer {
				underlying = underlying.Elem()
			}
			if field.Anonymous && field.Tag.Get("json") == "" && underlying.Kind() == reflect.Struct {
				_, optional := jsonField(field)
				add(underlying, inheritedOptional || optional || fieldType.Kind() == reflect.Pointer)
				continue
			}
			name, optional := jsonField(field)
			if _, duplicate := seen[name]; duplicate {
				panic("contractgen: colliding JSON field " + strconv.Quote(name) + " in " + typeOf.String())
			}
			seen[name] = struct{}{}
			optional = inheritedOptional || optional
			if optional && fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			fields = append(fields, contractField{name: name, typeOf: fieldType, optional: optional})
		}
	}
	add(typeOf, false)
	return fields
}

func contractFieldNames(fields []contractField) ([]string, []string) {
	var required, optional []string
	for _, field := range fields {
		if field.optional {
			optional = append(optional, field.name)
		} else {
			required = append(required, field.name)
		}
	}
	return required, optional
}

func propertyName(name string) string {
	for index, char := range name {
		if !(char == '_' || char == '$' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
			return strconv.Quote(name)
		}
	}
	return name
}

func optionalMark(optional bool) string {
	if optional {
		return "?"
	}
	return ""
}

func isContractDefinition(typeOf reflect.Type) bool {
	_, page := pageElement(typeOf)
	return !page && typeOf.Name() != "" && typeOf.PkgPath() != "" &&
		(typeOf.Kind() == reflect.Struct || typeOf.Kind() == reflect.String)
}

func pageElement(typeOf reflect.Type) (reflect.Type, bool) {
	if typeOf == nil {
		return nil, false
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.Kind() != reflect.Struct || typeOf.PkgPath() != reflect.TypeFor[protocol.Page[struct{}]]().PkgPath() || !strings.HasPrefix(typeOf.Name(), "Page[") {
		return nil, false
	}
	field, found := typeOf.FieldByName("Data")
	if !found || field.Type.Kind() != reflect.Slice {
		panic("contractgen: protocol.Page no longer has a Data slice")
	}
	return field.Type.Elem(), true
}
