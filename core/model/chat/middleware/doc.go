// Package middleware hosts opt-in [chat.CallMiddleware] /
// [chat.StreamMiddleware] decorators that lynx ships out of the box.
//
// Design ethos: every middleware in this package follows dependency
// inversion — the cross-cutting concern is fixed (logging, content
// safeguarding, ...) but the *how* is delegated to a small
// per-concern interface so user code can plug in whatever
// implementation makes sense for the deployment (slog vs zap vs OTel
// log bridge; substring vs regex vs ML classifier; ...). Lynx
// includes a thin default implementation of each interface backed by
// the standard library so the happy path is one line; advanced users
// override at the interface boundary.
//
// Layout:
//
//   - logger.go     — [Logger] interface, [NewLoggerMiddleware]
//     factory, and the slog-based default [NewSlogLogger].
//   - safeguard.go  — [Matcher] interface,
//     [NewSafeguardMiddleware] factory, and the
//     strings.Contains-based default [NewSubstringMatcher].
//
// Each factory returns the (call, stream) pair so a single
// registration covers both code paths.
package middleware
