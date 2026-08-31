# History-store adapters

Scope's history-store adapters connect persistence services to Core's provider-neutral history contracts.

## Scope

History-store adapters connect persistence services to Core's provider-neutral history contracts.

## Boundaries

Each backend remains an independently versioned leaf. The family owns persistence translation, not conversation semantics, product sessions, or retention policy.

Individual adapters document their own conceptual boundary. Their public APIs and executable usage live in GoDoc and checked examples.
