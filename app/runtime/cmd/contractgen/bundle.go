package main

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// bundle is the JSON Schema document: every wire type the protocol can carry, in
// one place, so no other artifact has to hold a second copy of a shape.
type bundle struct {
	Schema  string             `json:"$schema"`
	Title   string             `json:"title"`
	Version string             `json:"protocolVersion"`
	Defs    map[string]*schema `json:"$defs"`
}

// It carries no `$id`: without one, a relative `$ref` resolves against the file
// it appears in, which is what lets openrpc.json point at `schema.json#/$defs/X`
// instead of duplicating the definitions.
const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// walkWireTypes walks every type a client can send or receive: each method's
// params, its result, and — for a streaming method — its events. Downstream
// notification params are registered separately because they are not callable
// methods, but are equally part of the wire surface.
func walkWireTypes(registry *dispatch.Registry, shapes *dispatch.Shapes) *schemaSet {
	set := newSchemaSet(shapes)
	for _, contract := range toolset.PresentationContracts() {
		for enumType, values := range contract.EnumValues {
			set.registerEnum(enumType, values)
		}
		set.walk(contract.ResultType)
	}
	for _, meta := range registry.Metas() {
		set.walk(meta.Params)
		if meta.Result != nil {
			set.walk(meta.Result)
		}
		if meta.Event != nil {
			set.walk(meta.Event)
		}
	}
	for _, notification := range shapes.Notifications() {
		set.walk(notification.ParamsType)
	}
	// The union and constraint specs are registered against types the methods above
	// already reach; walking them again would be a no-op. Walk them anyway so a spec
	// for an unreachable type fails loudly here instead of silently describing
	// nothing.
	for _, union := range shapes.Unions() {
		set.walk(union.GoType)
	}
	for _, constraint := range shapes.Constraints() {
		set.walk(constraint.GoType)
	}
	// These two, by contrast, are reachable from NOTHING else — a carried shape rides
	// outside typed params and a state key's value rides an untyped map — so without
	// the declarations the published contract would simply omit them. Concrete tool
	// results are walked from toolset's own presentation contracts above.
	for _, carried := range shapes.Carried() {
		set.walk(carried.GoType)
	}
	for _, key := range shapes.StateKeys() {
		set.walk(key.PayloadType)
	}
	return set
}

func newBundle(set *schemaSet) bundle {
	return bundle{
		Schema:  schemaDialect,
		Title:   "Lyra Runtime Protocol wire types",
		Version: protocol.ProtocolVersion,
		Defs:    set.Definitions(),
	}
}
