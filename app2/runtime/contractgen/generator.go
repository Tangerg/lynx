// Package contractgen projects the canonical Go contract into checked-in artifacts.
package contractgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

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
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Operation    string   `json:"operation"`
	Idempotency  string   `json:"idempotency"`
	Pagination   string   `json:"pagination"`
	Params       string   `json:"params"`
	Result       string   `json:"result"`
	Errors       []string `json:"errors"`
	Materializes []string `json:"materializes"`
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
	encodedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("contractgen: encode manifest: %w", err)
	}
	encodedManifest = append(encodedManifest, '\n')
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
		methodManifest = append(methodManifest, MethodManifest{
			Name: method.Name, Kind: method.Kind.String(), Operation: method.Operation.String(),
			Idempotency: method.Idempotency.String(), Pagination: method.Pagination.String(),
			Params: typeName(method.Params), Result: typeName(method.Result),
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
	if typeOf == reflect.TypeFor[struct{}]() {
		return "EmptyObject"
	}
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
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
		for index := range typeOf.NumField() {
			field := typeOf.Field(index)
			if field.IsExported() && jsonName(field) != "-" {
				registry.add(field.Type)
			}
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
	}
	for _, endpoint := range httptransport.Contract() {
		registry.add(endpoint.ResponseType)
	}

	var output strings.Builder
	output.WriteString("/* Code generated by contractgen. DO NOT EDIT. */\n\n")
	fmt.Fprintf(&output, "export const protocolVersion = %s as const;\n\n", strconv.Quote(manifest.ProtocolVersion))
	output.WriteString("export type EmptyObject = Record<string, never>;\n\n")

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
		fmt.Fprintf(&output, "export interface %s {\n", name)
		for index := range typeOf.NumField() {
			field := typeOf.Field(index)
			fieldName, optional := jsonField(field)
			if !field.IsExported() || fieldName == "-" {
				continue
			}
			fieldType := field.Type
			if optional && fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			fmt.Fprintf(&output, "  %s%s: %s;\n", propertyName(fieldName), optionalMark(optional), registry.tsType(fieldType))
		}
		output.WriteString("}\n\n")
	}

	output.WriteString("export interface RuntimeMethods {\n")
	for _, method := range operation.Contract().Metas() {
		fmt.Fprintf(&output, "  %s: { params: %s; result: %s };\n", strconv.Quote(method.Name), registry.tsType(method.Params), registry.tsType(method.Result))
	}
	output.WriteString("}\n\n")
	output.WriteString("function isRecord(value: unknown): value is Record<string, unknown> {\n  return typeof value === \"object\" && value !== null && !Array.isArray(value);\n}\n\n")
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
			required, optional := fieldNames(typeOf)
			fmt.Fprintf(&output, "  if (!hasOnlyKeys(value, [%s], [%s])) return false;\n", quotedList(required), quotedList(optional))
			for index := range typeOf.NumField() {
				field := typeOf.Field(index)
				fieldName, isOptional := jsonField(field)
				if !field.IsExported() || fieldName == "-" {
					continue
				}
				fieldType := field.Type
				if isOptional && fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}
				expression := registry.validator(fieldType, "value["+strconv.Quote(fieldName)+"]")
				if isOptional {
					fmt.Fprintf(&output, "  if (%s in value && !(%s)) return false;\n", strconv.Quote(fieldName), expression)
				} else {
					fmt.Fprintf(&output, "  if (!(%s)) return false;\n", expression)
				}
			}
			output.WriteString("  return true;\n")
		}
		output.WriteString("}\n\n")
	}
	output.WriteString("export function assertDiscoverResponse(value: unknown): asserts value is DiscoverResponse {\n  if (!isDiscoverResponse(value)) throw new TypeError(\"invalid runtime.discover response\");\n}\n")
	return []byte(output.String()), nil
}

func (registry *typeRegistry) tsType(typeOf reflect.Type) string {
	if typeOf == reflect.TypeFor[struct{}]() {
		return "EmptyObject"
	}
	if typeOf.Kind() == reflect.Pointer {
		return registry.tsType(typeOf.Elem()) + " | null"
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

func (registry *typeRegistry) validator(typeOf reflect.Type, expression string) string {
	if typeOf == reflect.TypeFor[struct{}]() {
		return "isRecord(" + expression + ") && Object.keys(" + expression + ").length === 0"
	}
	if typeOf.Kind() == reflect.Pointer {
		return expression + " === null || " + registry.validator(typeOf.Elem(), expression)
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
  isDiscoverResponse,
  protocolVersion,
  type DiscoverResponse,
  type RequestMeta,
  type RuntimeMethods,
} from "./wire.generated.js";

export interface RuntimeConnection {
  endpoint: string;
  localToken: string;
  instanceId: string;
  protocolVersion: typeof protocolVersion;
  idempotencyNamespace: string;
  generation: number;
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

const responseValidators: {
  [Name in keyof RuntimeMethods]: (value: unknown) => value is RuntimeMethods[Name]["result"];
} = {
  "runtime.discover": isDiscoverResponse,
};

export class LyraClient {
  readonly #connection: RuntimeConnection;
  readonly #rpcURL: URL;

  constructor(connection: RuntimeConnection) {
    if (connection.protocolVersion !== protocolVersion) {
      throw new TypeError("Unsupported Runtime protocol " + connection.protocolVersion);
    }
    const endpoint = new URL(connection.endpoint);
    if (endpoint.protocol !== "http:" || endpoint.username !== "" || endpoint.password !== "") {
      throw new TypeError("Local Runtime endpoint must be an uncredentialed HTTP URL");
    }
    if (!["127.0.0.1", "[::1]"].includes(endpoint.hostname) || endpoint.pathname !== "/" || endpoint.search !== "" || endpoint.hash !== "") {
      throw new TypeError("Local Runtime endpoint must be an origin-only loopback URL");
    }
    this.#connection = { ...connection };
    this.#rpcURL = new URL("/v2/rpc", endpoint);
  }

  async discover(meta: RequestMeta = {}, signal?: AbortSignal): Promise<DiscoverResponse> {
    const response = await this.call("runtime.discover", {}, meta, signal);
    assertDiscoverResponse(response);
    return response;
  }

  async call<Name extends keyof RuntimeMethods>(
    method: Name,
    params: RuntimeMethods[Name]["params"],
    meta: RequestMeta = {},
    signal?: AbortSignal,
  ): Promise<RuntimeMethods[Name]["result"]> {
    const id = crypto.randomUUID();
    const requestParams = Object.keys(meta).length === 0 ? params : { ...params, _meta: meta };
    const response = await fetch(this.#rpcURL, {
      method: "POST",
      headers: {
        Authorization: "Bearer " + this.#connection.localToken,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ jsonrpc: "2.0", id, method, params: requestParams }),
      signal,
    });
    if (!response.ok || !response.headers.get("Content-Type")?.startsWith("application/json")) {
      throw new Error("Runtime transport failed with HTTP " + response.status);
    }
    const envelope: unknown = await response.json();
    if (!isRecord(envelope) || envelope.jsonrpc !== "2.0" || envelope.id !== id) {
      throw new TypeError("Runtime returned an invalid JSON-RPC envelope");
    }
    const keys = Object.keys(envelope);
    const hasResult = "result" in envelope;
    const hasError = "error" in envelope;
    if (hasResult === hasError || keys.some((key) => !["jsonrpc", "id", "result", "error"].includes(key))) {
      throw new TypeError("Runtime returned an ambiguous JSON-RPC envelope");
    }
    if (hasError) {
      const failure = envelope.error;
      if (!isRecord(failure) || !Number.isInteger(failure.code) || typeof failure.message !== "string") {
        throw new TypeError("Runtime returned an invalid JSON-RPC error");
      }
      throw new LyraRPCError(failure.code as number, failure.message, failure.data);
    }
    const validator = responseValidators[method];
    if (!validator(envelope.result)) {
      throw new TypeError("Runtime returned an invalid " + String(method) + " result");
    }
    return envelope.result;
  }
}
`)
	_ = manifest
	return []byte(output.String())
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
	return name, slices.Contains(options, "omitempty")
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

func fieldNames(typeOf reflect.Type) ([]string, []string) {
	var required, optional []string
	for index := range typeOf.NumField() {
		field := typeOf.Field(index)
		name, isOptional := jsonField(field)
		if !field.IsExported() || name == "-" {
			continue
		}
		if isOptional {
			optional = append(optional, name)
		} else {
			required = append(required, name)
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
	return typeOf.Name() != "" && typeOf.PkgPath() != "" &&
		(typeOf.Kind() == reflect.Struct || typeOf.Kind() == reflect.String)
}
