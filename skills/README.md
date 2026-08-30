# skills

`skills` is a read-only repository over
[Agent Skills](https://agentskills.io): directories that each hold a `SKILL.md`
(YAML frontmatter plus Markdown instructions) and optional bundled resources
under `references/`, `assets/`, and `scripts/`.

It parses, validates, and serves skill content. It does not execute scripts —
an agent runs those with its own shell and filesystem tools — and it knows
nothing about chat models or tools. The LLM-callable wrapper lives in
`tools/skills`, a thin adapter over `ResourceSource`.

## Install

```bash
go get github.com/Tangerg/scope/skills
```

## Progressive disclosure, in two interfaces

A skill is revealed in three steps, split across two interfaces so a caller
depends only on what it uses:

| Step | Method | Interface |
|---|---|---|
| Decide relevance from name and description | `List` | `Source` |
| Read one skill's full instructions | `Load` | `Source` |
| Open a bundled file on demand | `OpenResource` | `ResourceSource` |

```go
repository, err := skills.NewDirectoryRepository("./skills", skills.RepositoryConfig{})
if err != nil {
    return err
}

summaries, err := repository.List(ctx)
if err != nil {
    return err
}

skill, err := repository.Load(ctx, summaries[0].Name)
if err != nil {
    return err
}
```

Any `fs.FS` works through `NewRepository`; `NewDirectoryRepository` additionally
confines a real directory so resource resolution cannot escape it.

## Reading resources

`ReadResource` caps the read and reports whether the result was truncated. A
truncated body is valid content but must not be treated as the whole file:

```go
content, truncated, err := skills.ReadResource(
    ctx, repository, "pdf-processing", "references/REFERENCE.md",
    skills.DefaultMaxResourceBytes,
)
```

Resource paths are anchored inside the owning skill directory. `..` traversal,
absolute paths, backslashes, and symlinks that leave the skill are all refused —
this is a trust boundary, not a convenience check.

## Merging sources

`Merge` layers sources by precedence. The unit of precedence is the whole
bundle: once a skill wins, its resources come only from the source that owns it,
so a lower-precedence copy with the same name can never contribute files.

## Reading is lazy

Nothing is cached or pre-scanned. An external edit is visible on the next call.
Caching and preloading are the caller's decision.

A malformed skill is skipped during `List` — a missing `SKILL.md`, an illegal
directory name, or content that violates the specification — which matches the
wider ecosystem's behavior. A repository access failure such as a permission or
media error is returned instead, never disguised as an empty list.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants behind these rules.
