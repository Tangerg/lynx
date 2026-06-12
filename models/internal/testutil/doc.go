// Package testutil provides shared test helpers for model vendors.
//
// The package is intentionally internal so it can grow vendor-specific
// helpers without polluting the public API. The helpers fall into three
// categories:
//
//   - env: pull API keys from environment with consistent naming
//   - sse: spin up httptest servers that produce SSE / chunked-JSON
//     responses that mirror real provider wire formats
//   - stream: drain iter.Seq2 iterators with cancellation helpers
package testutil
