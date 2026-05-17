package cohere_test

// Mock-based unit tests for Cohere are not provided in this package.
//
// The Cohere v2 Go SDK panics when its internal RequestOptions is
// constructed without a fully initialized HTTPHeader — its
// ToHeader / cloneHeader path nil-derefs when invoked against a bare
// httptest.Server. The crash is inside the SDK and we can't reach
// past it without monkey-patching. Until upstream is fixed, run the
// integration test (chat_integration_test.go) against the real API
// to exercise the path.
