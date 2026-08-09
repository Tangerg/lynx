// Package domain contains the product's bounded contexts: one sub-package per
// business capability, holding entities, value objects, aggregates, and pure
// domain policies. Domain packages are independent of orchestration use cases,
// transport, storage, and integration mechanisms. I/O ports live outside this
// layer with their consuming use case; Domain may define only pure strategy
// contracts whose implementations require no I/O.
package domain
