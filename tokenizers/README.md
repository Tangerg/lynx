# Tokenizer adapters

Scope's tokenizer adapters implement Core's provider-neutral tokenizer
contracts with concrete vocabularies. This directory is a namespace, not a
module: every adapter below it is an independently versioned leaf with its own
`doc.go`.

## Ownership

- Each leaf owns only its vocabulary, configuration, and translation.
- Core owns the portable counting and encoding contracts.
- The family owns vocabulary translation, not budget policy, context-window
  policy, or product accounting.

## Dependencies

- A leaf may depend on Core and the vocabulary library it adapts.
- Leaves do not import sibling adapters or higher-level capability modules.

## Invariants

- A vocabulary is chosen explicitly; no adapter guesses an encoding on the
  caller's behalf.
- External details do not alter or leak through shared contracts.
- Construction, authority, and unsupported capabilities are explicit.
- Runtime policy and product workflows remain outside adapter modules.

Each adapter's own boundary, public API, and executable usage live in its
`doc.go`, GoDoc, and checked examples.
