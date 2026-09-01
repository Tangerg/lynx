# GitNexus: adjacent-system evidence, not an agent framework peer

Evidence baseline: `GitNexus` commit
`b059ab3541ea68c2ce292955fc367a5de04b39ea`.

## Why it left the primary matrix

GitNexus's core responsibility is indexing code, building a relationship graph,
running code retrieval, and returning context to a tool or an interface. It has
no agent execution contract, model-tool loop, child execution lifecycle, or
recovery kernel comparable to Scope, Pi, or Eino.

An earlier draft conceded it was not an agent framework and still scored it on
the eight dimensions, which produced meaningless comparisons: "no agent
protocol" is not a defect for GitNexus, it is a product boundary.

## Evidence that remains useful to Scope

### A tool result needs to distinguish tiers of fact

What code retrieval returns may include:

- symbols and relationships confirmed directly from syntax or an index;
- associations derived through a graph path;
- explanations produced by natural-language summarization;
- relationships missing because the index is incomplete.

When an agent framework flattens all of these into plain text, the model cannot
reliably separate a verified fact from an inference. Scope's general `Part` and
tool-result extensions should let a host preserve provenance, confidence
source, index version, and truncation information — without writing a
GitNexus-specific schema into the core.

### A large result needs explicit budget and truncation semantics

A code-graph query easily exceeds a context budget. A good tool protocol should
at least express:

- whether the result is complete;
- what scope and filters were used;
- whether truncation or pagination occurred;
- how to continue the query.

Pi's choice to refuse execution on a truncated tool-call argument reflects the
same principle: an incomplete protocol must not masquerade as complete input.

### Index lifecycle belongs to the tool implementation

Index construction, incremental refresh, caching, and graph storage do not
belong in an agent execution kernel. Scope only needs to provide a cancellable,
observable tool Effect with an identity; GitNexus owns its own index
consistency.

## What must not be inferred from GitNexus

- Repository size, a UI, an MCP server, or a retrieval count does not measure
  agent framework maturity.
- Scope lacking a code knowledge graph does not indicate a missing framework
  capability.
- GitNexus's storage model must not be copied into a general agent memory.
- A tool's internal checkpoint must not be conflated with an agent Execution
  snapshot.

## Final placement

GitNexus is worth studying as a potential tool or adjacent infrastructure in
the Scope ecosystem. Its most important lesson for the framework is evidence
truthfulness, result truncation, and provenance — not a ranking of agent kernel
design.
