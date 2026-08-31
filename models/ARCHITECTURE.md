# Model adapters architecture

This family groups independently versioned adapters behind shared Core contracts.

## Ownership

- Each leaf owns only its external boundary, configuration, and translation.
- Core owns portable semantics and contract tests.

## Dependencies

- A leaf may depend on Core and the external service it adapts.
- Leaves do not import sibling adapters or higher-level capability modules.

## Invariants

- External details do not alter or leak through shared contracts.
- Every portable option has one Core-owned representation; adapters do not add provider-specific synonyms for the same capability.
- Adapters translate a portable semantic exactly or reject it before provider I/O; they never silently weaken an unsupported request.
- Complete provider outputs and stream events map to their corresponding Core values without manufacturing partial stable content.
- Construction, authority, provider-only extensions, and unsupported capabilities are explicit.
- Runtime policy and product workflows remain outside adapter modules.
