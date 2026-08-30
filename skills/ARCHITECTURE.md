# skills architecture

> The read-only repository layer over Agent Skills: parse, validate, and serve
> `SKILL.md` directories on demand. It is the backend tier of the two-tier tools
> SPI, sitting alongside the shell and filesystem executors.

Repository-wide rules live in [`../CLAUDE.md`](../CLAUDE.md). Field-level rules
follow the [agentskills.io](https://agentskills.io) specification; the usage
entry point is [`README.md`](README.md).

---

## 1. Position

A pure capability layer. It reads a skill directory out — parsing, validating,
and fetching content on demand. It does not execute scripts (the agent's own
shell and filesystem tools do that) and it does not know about chat or tools
(the wrapper lives in `tools/skills`).

## 2. Mental model

- **Three disclosure levels, two interfaces.** List (name plus description —
  enough to judge relevance) → load one skill's full instructions → open a
  bundled resource on demand. A caller that only needs the first two levels
  depends on the narrow `Source`; one that needs files depends on the extended
  `ResourceSource`. This is the module's ISP boundary.
- **Zero domain dependencies.** It imports no `core`, `agent`, or `chat`, and
  the three levels return plain Go types. That keeps it at the bottom of the
  dependency graph.
- **The specification is the truth.** Field rules — including that a skill's
  name must equal its directory name — follow agentskills.io. Validation reports
  every violation at once rather than stopping at the first.
- **Content only, never execution.** A script resource hands over its content;
  running it belongs to the agent's existing tools. This is KISS, not building a
  second shell, and a security boundary.
- **Paths cannot escape.** Resource opening is anchored inside the skill
  directory. `..` traversal is refused, and so is a symlink that leaves either
  the skill or the repository root. This is a trust boundary.
- **Precedence applies to the whole bundle.** When a higher-precedence skill
  wins a `Merge`, its resources come only from that same source. A
  lower-precedence skill with the same name must not contribute files.
- **Context is part of the I/O contract.** `List`, `Load`, `OpenResource`, and
  `ReadResource` observe cancellation before and after access. A custom `Source`
  must do the same.
- **A bad skill does not break the list.** Listing skips entries with a missing
  `SKILL.md`, an illegal directory name, or non-conforming content. A repository
  access failure — permission, media, or remote filesystem — is returned, never
  disguised as an empty list.
- **Reads are lazy and per call.** Nothing is cached or pre-scanned, so an
  external edit is immediately visible. Caching and preloading belong to the
  caller.

## 3. Negative invariants

- Never import `core`, `chat`, or any domain module. This is a bare capability
  layer; the `tool.Tool` adaptation lives in `tools/skills`.
- Never execute a script, introduce a sandbox, or reach for a container here.
  Script execution belongs to the agent's existing tools.
- Never enforce `allowed-tools`. It is an experimental field: parse it and hand
  it to a caller that chooses to execute.
- Never add a cache, a pub/sub channel, or state management. This module is
  read-only; a caller that needs those wraps it.

## 4. Read before changing

- Changing what the model sees — operations, schema, rendering — belongs to
  `tools/skills`, not here.
- Changing `Source` or `ResourceSource` is a consumer contract change that
  reaches `tools/skills` and every custom implementation.
- A specification upgrade changes the frontmatter structure and its validation;
  check against the official specification.
- A new backend (remote skill store, embedded filesystem) implements the narrow
  interface for list and load, and the extended one only if it serves resources.
