# Vector-store adapters

Scope's vector-store adapters connect storage services to Core's provider-neutral vector-store contracts.

## Scope

Vector-store adapters connect storage services to Core's provider-neutral vector-store contracts.

## Boundaries

Each backend remains an independently versioned leaf. The family owns backend translation, not retrieval semantics, embedding policy, or product search.

Individual adapters document their own conceptual boundary. Their public APIs and executable usage live in GoDoc and checked examples.
