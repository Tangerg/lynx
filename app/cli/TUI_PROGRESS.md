# Lyra TUI progress baseline

This ledger is the acceptance baseline for the CLI-only TUI refinement goal.
It records product behavior, not implementation tasks, so refactors and breaking
changes do not make completed evidence ambiguous.

Baseline date: 2026-08-11
Lynx baseline: `451e839fa`
Oolong baseline: `v0.11.0`
Current Oolong release: `v0.12.0`

Reference snapshots:

- agentui `ec0414e1e3dd`
- opencode `89130db6b006`
- kimi-code `c27a9f93a653`
- grok-build `780d1388fff1`
- codex `e4e040881aca`
- claude_code `f25304f0cce6` (interaction reference only; unofficial snapshot)

## Scope and constraints

- Only `app/cli` may change for this goal.
- `app/runtime` remains authoritative for execution, persistence, recovery,
  approval semantics, and protocol-representable state.
- CLI-owned presentation, navigation, drafts, local history, notifications,
  command discovery, and terminal contract tests may evolve independently.
- Breaking CLI changes are allowed. No compatibility shims, state migrations,
  or duplicate legacy APIs are retained.
- Oolong owns terminal primitives and text editing mechanics. Lyra owns product
  policy and must not fork those primitives locally.
- Every completed row requires deterministic tests. Terminal protocol behavior
  additionally requires a real PTY contract test.

Status values are `done`, `active`, `pending`, and `deferred`.

## Capability ledger

| ID | Capability | Baseline | Target evidence | Status |
| --- | --- | --- | --- | --- |
| PTY-01 | Terminal lifecycle and mode restoration | Binary PTY coverage exists | xterm, screen, VS Code/WSL modes remain symmetric | done |
| PTY-02 | Submission and multiline input | In-memory coverage | Real PTY covers fast submit, Shift/Alt+Enter, UTF-8 and paste boundaries | done |
| PTY-03 | Streaming navigation | In-memory coverage | Real PTY covers scroll suspension, tool expansion, approval and resize while streaming | done |
| VIEW-01 | Inline semantic tool blocks | Implemented | Existing tests remain authoritative | done |
| VIEW-02 | Full-content reader | Inline output is capped | Searchable full-screen reader with stable selection and live-tail policy | done |
| VIEW-03 | Tool presentation registry | One generic block presenter | Semantic presenters with ordered matching and generic fallback | done |
| VIEW-04 | Adjacent tool grouping | Each tool is independent | Foldable groups preserve each tool identity and lifecycle | done |
| HITL-01 | Approval and question decisions | Implemented sequential dialogs | Rememberability is enforced; offered and custom answers compose without losing option metadata; validation and resize preserve editable state | done |
| HITL-02 | Tool-aware approval preview | Diff-first generic presentation | Shell, edit, write, read and network requests use deterministic presentations | done |
| HITL-03 | Denial feedback | Fixed denial reason | Optional user feedback is submitted as the denial reason | done |
| HITL-04 | Interaction review | Answers commit one dialog at a time | Multi-item wizard supports back, edit, review and one final resume | done |
| INPUT-01 | Rich editor mechanics | Oolong v0.11 editor | Grapheme movement, selection, undo and kill-ring behavior remain delegated | done |
| INPUT-02 | Durable prompt history | Process memory only | Crash-safe bounded history is shared across launches | done |
| INPUT-03 | Session draft | No durable draft | Text and attachment identities restore per session | done |
| INPUT-04 | Prompt stash | No stash | Stash, list, apply and delete are explicit CLI-owned operations | done |
| INPUT-05 | External editor | No round trip | Configured editor round trip preserves the original draft on failure | done |
| INPUT-06 | Run admission ordering | Async runtime mutations could race the next prompt | Run-affecting mutations own an explicit admission barrier; prompts persist behind it and drain automatically | done |
| SESSION-01 | Session switch/create/rename/fork | Implemented | Existing behavior remains authoritative | done |
| SESSION-02 | Paginated session center | First page only | Cursor pagination, grouping, preview, favorite, rename and delete | done |
| SESSION-03 | Current-session timeline | No dedicated surface | Jump to retained entries and fork from an existing root run | done |
| SESSION-04 | Runtime rewind/rollback | Runtime protocol is now exposed | Consume authoritative rollback with preview, confirmation, and recovery tests | done |
| CMD-01 | Searchable command palette | Implemented flat catalog | Existing palette remains authoritative | done |
| CMD-02 | Context-aware command catalog | Handlers reject unavailable actions late | Category, availability and disabled reason share one descriptor | done |
| CMD-03 | Pending key sequence hint | No focused hint | Only an active chord shows valid continuations | done |
| NOTICE-01 | Run completion notifications | Implemented without focus policy | Focus-aware approval, question, failure and completion notifications | done |
| NOTICE-02 | Terminal title state | Static session title | Unfocused action-required state is reflected and later cleared | done |
| OUTPUT-01 | Transcript copy/export/import | Selected block copy only | Last assistant copy plus runtime-native Markdown/JSON export and portable JSON import | done |
| RUN-01 | Exact-segment steering | Follow-ups are queue-only | Steer text/attachments bind to the observed segment and fail closed when stale | done |
| WORKSPACE-01 | Workspace selection | New sessions inherit current workspace | Recent workspace picker and explicit directory selection | done |
| WORKSPACE-02 | Runtime workspace inspection | Local attachment resolver only | Runtime-backed changes, diff, head, list, read and grep surfaces use the full reader | done |
| WORKSPACE-03 | File invalidation stream | No runtime-wide subscription | Negotiated watch refetches authoritative changes after events, gaps and reconnects | done |
| BACKEND-02 | Session-side invalidation stream | No side-channel reconciliation | Session, run, state and interrupt events trigger scoped authoritative reads without racing active streams | done |
| BACKEND-03 | Runtime management surfaces | Usage, provider, auxiliary-role, and goal APIs were not consumed | Secret-safe provider configuration, usage reporting, model roles, goal lifecycle, and goal invalidation have deterministic and real-runtime evidence | done |
| BACKEND-04 | Skill governance surfaces | Skill APIs and `skills.changed` were not consumed | Workspace discovery, managed lifecycle, immutable proposal review, resize-safe confirmation, and authoritative invalidation refresh | done |
| BACKEND-05 | MCP connection surfaces | MCP APIs and `mcp.changed` were not consumed | Secret-safe server lifecycle, transport-specific bounded wizard, tool schemas, probes, reconnect, browser authorization polling, resize-safe editing, and authoritative invalidation refresh | done |
| BACKEND-06 | Scheduled-run surfaces | Schedule APIs and `schedules.changed` were not consumed | Cursor-complete catalog, revision-guarded editing, enable/disable, immediate run handles, destructive confirmation, resize-safe forms, and authoritative invalidation refresh | done |
| BACKEND-07 | Governed agent memory | Agent memory APIs were not consumed | Project/user partitions, provenance, pending review, pin/edit/add, confirmed decisions/deletion, resize-safe multiline authoring, and authoritative post-mutation reads | done |
| BACKEND-08 | Human-authored knowledge | Knowledge APIs were not consumed | LYRA.md cascade list/get/update, exact scope context, verbatim multiline editing/clearing, resize safety, authoritative post-save reads, and external-edit invalidation | done |
| BACKEND-09 | Diagnostics, authoring context, hooks, and feedback | Seven exported embedded APIs were not consumed | Safe workspace-confined direct tools, agent documents, recipe expansion, hook trust governance with external-edit invalidation, and scoped feedback have adapter, terminal, resize, and real-runtime evidence | done |
| BACKEND-01 | Public runtime API coverage | Core run/session methods only | Every exported embedded API is inventoried and tracked to a consumer surface and test | done |
| QUALITY-01 | Deterministic package tests | Full suite passes | New domains use table tests and consumer-owned fakes | done |
| QUALITY-02 | Race safety | Terminal race suite passes | Full CLI race suite passes after all batches | done |
| QUALITY-03 | Architecture boundaries | Architecture tests exist | New packages do not import runtime protocol, Cobra, Viper, or Oolong inward | done |

## Delivery order

1. Terminal contract tests.
2. Reader and tool presentation.
3. HITL presentation and interaction review.
4. Durable prompt workbench.
5. Session center and timeline.
6. Command discovery and focus-aware notifications.
7. Export, workspace polish, architecture review, race tests, and final cleanup.

The ledger is updated only when the target evidence exists; implementation
progress without its acceptance evidence remains `active` or `pending`.

## Final verification

The CLI-only implementation passed these gates on 2026-08-12:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `golangci-lint run ./...` (`0 issues`)
- `staticcheck ./...`
- `go build ./...`
- `git diff --check -- app/cli`

The root package exercises the published Oolong PTY adapter through a built
binary; package tests cover the presentation, interaction, workbench, export,
session, workspace, attention, and architecture contracts. Runtime-native
session portability, rollback, and exact-segment steering are covered by
protocol-shape tests, real embedded round trips, destructive-dialog resize
tests, stale-segment failure tests, and authoritative cold reinstall. Runtime
invalidation tests cover attach-first cold reads, limit-aware partitioning,
topic negotiation, scope, sequence-gap resync, metadata-only updates,
side-channel session refresh, and goal, Skill, MCP, knowledge, and hook
projection refresh. Provider and MCP configuration tests
additionally prove that write-only credentials never appear in recorded
terminal frames, including after an extreme resize. The final backend batch
adds fail-closed diagnostic JSON/safety validation, semantic-index projection
tests, recipe substitution and unified prompt dispatch, resize-safe reindex and
hook-trust confirmation, durable feedback targeting, and live embedded reads and
writes for the stable auxiliary services.
