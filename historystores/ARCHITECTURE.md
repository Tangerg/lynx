# History-store adapters architecture

This family groups independently versioned adapters behind shared Core contracts.

## Ownership

- Each leaf owns only its external boundary, configuration, and translation.
- Core owns portable semantics and contract tests.

## Dependencies

- A leaf may depend on Core and the external service it adapts.
- Leaves do not import sibling adapters or higher-level capability modules.

## Invariants

- External details do not alter or leak through shared contracts.
- Construction, authority, and unsupported capabilities are explicit.
- Runtime policy and product workflows remain outside adapter modules.
