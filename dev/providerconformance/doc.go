// Package providerconformance checks the cross-provider consistency that no
// single provider module can check for itself: that every provider constructs
// the same way, exposes the same shape, and classifies failures with the same
// vocabulary.
//
// It is development tooling and declares no API. No product module depends on
// it, and nothing here ships.
//
// A provider module runs the behavior suite from core/modeltest against its own
// implementation, which proves one provider obeys the protocol. It cannot prove
// that the whole family agrees on construction, naming, and error
// classification — that needs all providers visible at once, which is exactly
// what a provider module must not do, since providers never import siblings.
//
// A new provider joins these checks by existing. If adding one makes a check
// fail, the provider is inconsistent with the family: fix the provider before
// relaxing the check.
//
// See README.md for how to run it and ARCHITECTURE.md for what it owns.
package providerconformance
