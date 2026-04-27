// Package json provides two helpers built on top of encoding/json:
//
//   - JSON Schema generation from Go values via [StringDefSchemaOf] /
//     [MapDefSchemaOf]; configurable through [SchemaConfig].
//   - Incremental JSON stream parsing via [StreamParser], which fires
//     callbacks for each completed top-level array, object, or scalar.
package json
