package main

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// The OpenRPC document is the method surface: what a client may call, what it
// passes, what comes back, and which errors are in scope. Shapes are NOT copied
// into it — every schema is a reference into schema.json, so there is one home for
// a wire type and a client tool reads the same bytes the validator does.

// openrpcVersion is the spec revision this document conforms to.
const openrpcVersion = "1.3.2"

// bundleRef is the sibling document holding the shapes. A relative reference
// resolves against this file's own location, which is why the bundle carries no
// $id to redirect the base.
const bundleRef = "schema.json"

type openrpcDocument struct {
	OpenRPC       string                `json:"openrpc"`
	Info          openrpcInfo           `json:"info"`
	Methods       []openrpcMethod       `json:"methods"`
	Notifications []openrpcNotification `json:"x-lyra-notifications"`
}

type openrpcNotification struct {
	Name   string  `json:"name"`
	Params *schema `json:"params"`
}

type openrpcInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openrpcMethod struct {
	Name string `json:"name"`

	// ParamStructure is by-name for every method: the wire passes one JSON object
	// whose keys are the request's own fields (API.md §2.5), so each field is a
	// named param rather than a positional one.
	ParamStructure string         `json:"paramStructure"`
	Params         []openrpcParam `json:"params"`
	Result         *openrpcResult `json:"result,omitempty"`
	Errors         []openrpcError `json:"errors,omitempty"`

	// The x-lyra extensions carry what OpenRPC has no vocabulary for: retry
	// semantics, cursor pagination, capability gating, and the event stream a
	// streaming method's response body becomes.
	Kind         string          `json:"x-lyra-kind"`
	Operation    string          `json:"x-lyra-operation"`
	Idempotency  string          `json:"x-lyra-idempotency"`
	Pagination   string          `json:"x-lyra-pagination"`
	Stability    string          `json:"x-lyra-stability"`
	Features     []string        `json:"x-lyra-features,omitempty"`
	Capabilities []capabilityRow `json:"x-lyra-capabilityRules,omitempty"`
	StreamEvent  *schema         `json:"x-lyra-streamEvent,omitempty"`

	// RequestFrame references the whole params object. By-name params describe the
	// fields one at a time and so cannot express a cross-field rule; the frame
	// schema is where those live, and pointing at it beats restating them.
	RequestFrame *schema `json:"x-lyra-requestFrame"`
}

type openrpcParam struct {
	Name     string  `json:"name"`
	Required bool    `json:"required,omitempty"`
	Schema   *schema `json:"schema"`
}

type openrpcResult struct {
	Name   string  `json:"name"`
	Schema *schema `json:"schema"`
}

// openrpcError pairs the symbolic problem type a client branches on with the
// numeric code the envelope carries. Both come from the error registry, so a
// method cannot document an error the runtime has no code for.
type openrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newOpenRPC(registry *dispatch.Registry, shapes *dispatch.Shapes, set *schemaSet) openrpcDocument {
	codes := problemCodes(registry)
	document := openrpcDocument{
		OpenRPC: openrpcVersion,
		Info: openrpcInfo{
			Title:       "Lyra Runtime Protocol",
			Version:     protocol.ProtocolVersion,
			Description: "Generated from the Contract Registry. Shapes live in schema.json; this document is the method surface.",
		},
	}
	for _, meta := range registry.Metas() {
		document.Methods = append(document.Methods, openrpcMethodFor(meta, set, codes))
	}
	for _, notification := range shapes.Notifications() {
		document.Notifications = append(document.Notifications, openrpcNotification{
			Name:   notification.Name,
			Params: external(set.walk(notification.ParamsType)),
		})
	}
	return document
}

func openrpcMethodFor(meta dispatch.MethodMeta, set *schemaSet, codes map[string]int) openrpcMethod {
	method := openrpcMethod{
		Name:           meta.Name,
		ParamStructure: "by-name",
		Params:         []openrpcParam{},
		Kind:           meta.Kind.String(),
		Operation:      meta.Operation.String(),
		Idempotency:    meta.Idempotency.String(),
		Pagination:     meta.Pagination.String(),
		Stability:      string(meta.Stability),
		Features:       meta.Features(),
		Capabilities:   capabilityRowsFor(meta),
		RequestFrame:   external(set.walk(meta.Params)),
	}
	for _, field := range protocol.WireFields(meta.Params) {
		method.Params = append(method.Params, openrpcParam{
			Name:     field.Name,
			Required: !field.Optional,
			Schema:   external(set.walk(field.Type)),
		})
	}
	result := &schema{Type: schemaTypeObject}
	if meta.Result != nil {
		result = set.walk(meta.Result)
		if meta.ResultNullable {
			result = &schema{AnyOf: []*schema{result, {Type: schemaTypeNull}}}
		}
	}
	method.Result = &openrpcResult{Name: "result", Schema: external(result)}
	if meta.Event != nil {
		method.StreamEvent = external(set.walk(meta.Event))
	}
	for _, problem := range meta.ProblemTypes() {
		code, ok := codes[problem]
		if !ok {
			panic("contractgen: " + meta.Name + " declares problem type " + problem + ", which the error registry has no code for")
		}
		method.Errors = append(method.Errors, openrpcError{Code: code, Message: problem})
	}
	return method
}

func capabilityRowsFor(meta dispatch.MethodMeta) []capabilityRow {
	if len(meta.CapabilityRules) == 0 {
		return nil
	}
	rows := make([]capabilityRow, 0, len(meta.CapabilityRules))
	for _, rule := range meta.CapabilityRules {
		rows = append(rows, capabilityRow{When: conditions(rule.When), Requires: rule.Requires})
	}
	return rows
}

// external re-points every reference in a walked schema at the shape bundle.
//
// It has to be a deep copy, not a rewrite: a reference can be nested — a param of
// type []ContentBlock walks to an array whose items reference the bundle — and the
// two documents must stay independent, so rendering one can never leave a
// bundle-local reference in the other or a foreign one in the bundle.
func external(node *schema) *schema {
	if node == nil {
		return nil
	}
	out := *node
	if out.Ref != "" {
		out.Ref = bundleRef + out.Ref
	}
	out.Items = external(node.Items)
	out.If = external(node.If)
	out.Then = external(node.Then)
	out.OneOf = externalAll(node.OneOf)
	out.AnyOf = externalAll(node.AnyOf)
	out.AllOf = externalAll(node.AllOf)
	if node.Properties != nil {
		out.Properties = make(map[string]any, len(node.Properties))
		for name, value := range node.Properties {
			if child, ok := value.(*schema); ok {
				out.Properties[name] = external(child)
				continue
			}
			// A forbidden field is the boolean schema `false`; it holds no reference.
			out.Properties[name] = value
		}
	}
	if child, ok := node.AdditionalProps.(*schema); ok {
		out.AdditionalProps = external(child)
	}
	// Required and Enum are read-only in both documents, so the slices are shared.
	return &out
}

func externalAll(nodes []*schema) []*schema {
	if nodes == nil {
		return nil
	}
	out := make([]*schema, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, external(node))
	}
	return out
}
