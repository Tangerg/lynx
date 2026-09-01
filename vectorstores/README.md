# Vector-store adapters

Scope's vector-store adapters connect storage services to Core's
provider-neutral vector-store contracts. This directory is a namespace, not a
module: every adapter below it is an independently versioned leaf with its own
`doc.go`.

## Ownership

- Each leaf owns only its external boundary, configuration, and translation.
- Core owns portable semantics and the contract test suites.
- The family owns backend translation, not retrieval semantics, embedding
  policy, or product search.

## Dependencies

- A leaf may depend on Core and the external service it adapts.
- Leaves do not import sibling adapters or higher-level capability modules.

## Invariants

- External details do not alter or leak through shared contracts.
- Construction, authority, and unsupported capabilities are explicit.
- Runtime policy and product workflows remain outside adapter modules.

Each adapter's own boundary, public API, and executable usage live in its
`doc.go`, GoDoc, and checked examples.
