// Package operation owns Runtime's binding-neutral typed operation catalog and
// execution policies. HTTP and in-process bindings both enter through this
// boundary so validation, capability gating, idempotency, error projection and
// event filtering have one implementation.
package operation
