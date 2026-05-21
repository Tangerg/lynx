// Package options carries the small generic helpers vendor adapters
// in /models share for safely extracting Extra-threaded SDK params
// out of [chat.Options] / [embedding.Options] / etc.
//
// The canonical use: a vendor's Extra map holds `*SomeNativeRequest`
// keyed by the package's OptionsKey constant. [GetParams] returns
// that value when present, or a fresh zero-value `*T` when absent
// — so adapter code can write `req := options.GetParams[Foo](opts,
// FooKey)` without nil-checking on every field access.
//
// Internal: not part of the public lynx API.
package options
