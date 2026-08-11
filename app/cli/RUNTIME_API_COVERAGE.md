# Runtime API consumption ledger

This is the progress contract for consuming the public in-process runtime API
from `app/cli`. It is generated from the exported methods of
`app/runtime/embedded.Runtime`, then reviewed by bounded context. Public runtime
protocol DTOs remain confined to `internal/runtimeembedded`; every other package
uses a CLI-owned domain model and a consumer-owned port.

Baseline date: 2026-08-12
Runtime API inventory: 87 exported methods
Production API consumption: 87 methods
Queued API consumption: 0 methods

Status meanings:

- `complete`: a production CLI path calls the method, projects the result, and
  has deterministic acceptance evidence.
- `partial`: the context is wired, but one or more methods, topics, or user
  surfaces in the row remain.
- `queued`: the runtime exposes the method, but this CLI has no production
  consumer yet.

No row is complete merely because the adapter compiles. Query capabilities
need a CLI/TUI surface; invalidation topics need an authoritative refetch and
resync policy; mutations need lifecycle and failure-path tests.

| Bounded context | Exported embedded methods | Status | Current consumption and remaining work |
| --- | --- | --- | --- |
| Runtime lifecycle and discovery | `Discover`, `Close` | complete | Open validates protocol, run stream vocabulary, replay scope, plan recovery, topics, and closes process-owned state. |
| Runtime invalidations | `SubscribeRuntime` | complete | One negotiated attach-first subscription consumes every advertised topic: `files.changed`, `skills.changed`, `mcp.changed`, `schedules.changed`, `sessions.changed`, `runs.changed`, `state.changed`, `goals.changed`, and `interrupts.changed`. File events re-read workspace changes; session/run/state/interrupt events re-read the authoritative session without taking ownership from an active stream; Skill, MCP, schedule, and goal events refresh only their open projections. Reconnects and sequence gaps resync every subscribed topic. |
| Sessions | `CreateSession`, `DeleteSession`, `ForkSession`, `GetSession`, `ListSessions`, `UpdateSession` | complete | Interactive session center, switching, creation, rename, favorite, fork, delete, and cold snapshot recovery. |
| Session portability and rewind | `ExportSession`, `ImportSession`, `RollbackSession` | complete | Runtime-authored Markdown/JSON exports, opaque JSON round-trip import, conflict-safe artifact files, authoritative rollback preview, destructive confirmation, change-before-commit rejection, and cold snapshot reinstall. |
| Runs | `CancelRun`, `GetRun`, `ListRuns`, `ResumeRun`, `StartRun`, `SubscribeRun` | complete | Core streaming, reconnect/replay, recovery, cancellation, HITL resume, and timeline. `GetRun`/`ListRuns` are consumed by the cold session projection. |
| Run steering | `SteerRun` | complete | `/steer` binds text and staged attachments to the exact observed run/segment; stale segments fail closed and refused attachments return to the live draft. Queued follow-ups remain a distinct interaction. |
| Run resources | `GetPlan`, `ListInterrupts`, `ListItems` | complete | Folded into the authoritative cold snapshot and recovery/HITL projections. |
| Models | `ListModels` | complete | Provider-qualified model picker and run options. |
| Model roles | `GetEmbeddingRole`, `GetUtilityRole`, `SetEmbeddingRole`, `SetUtilityRole` | complete | `/roles`, `/utility`, and `/embedding` inspect and mutate the two auxiliary roles without conflating them with the primary run model. |
| Providers | `ListProviders`, `TestProvider`, `UpdateProvider` | complete | `/providers`, `/provider-test`, and `/provider-config` expose status, endpoint/key changes, structured diagnostics, resize-safe editing, and masked write-only secret handling. |
| Approvals | `ForgetApprovalRule`, `GetApprovalMode`, `ListApprovalRules`, `SetApprovalMode` | complete | Approval mode picker, remembered-rule catalog/deletion, and HITL decisions. |
| Workspace catalog | `ListWorkspaces`, `ResolveWorkspace` | complete | Runtime-known workspace inspector and picker; explicit workspace changes resolve through the authoritative runtime service. |
| Workspace inspection | `GetWorkspaceDiff`, `GetWorkspaceFileHead`, `ListWorkspaceFileChanges`, `ListWorkspaceFiles`, `ReadWorkspaceFile`, `SearchWorkspaceFiles` | complete | `/diff`, `/preview`, `/changes`, `/browse`, `/read`, and `/grep`, all rendered in the searchable full reader. |
| Usage | `GetSessionUsage`, `GetUsageSummary` | complete | `/usage [positive-days\|all]` renders session and global totals plus provider/model/day breakdowns; live stream usage remains the active-run source. |
| Goals | `GetGoal`, `ResumeGoal`, `StartGoal`, `StopGoal` | complete | `/goal`, `/goal-start`, `/goal-stop`, and `/goal-resume` expose objective, model, budget, usage, reason, and the full lifecycle; `goals.changed` refetches an open goal projection. |
| Agent memory | `AddAgentMemory`, `DeleteAgentMemory`, `ListAgentMemory`, `ReviewAgentMemory`, `UpdateAgentMemory` | complete | `/memory` and the `memory-*` commands preserve project/user partitions, provenance, pending review state, exact item identity, pinning, multiline edits, and confirmed review/deletion; every successful mutation immediately re-reads the selected scope because the runtime exposes no memory invalidation topic. |
| Knowledge | `GetKnowledge`, `ListKnowledge`, `UpdateKnowledge` | complete | `/knowledge`, `/knowledge-read`, and `/knowledge-edit` expose the human-authored `cwd`/`projectRoot`/`home` LYRA.md cascade in a multiline resize-safe editor, preserve content verbatim (including intentional clearing), omit irrelevant workspace context for home scope, and read the saved document back authoritatively. The runtime exposes no knowledge invalidation topic. |
| Skills | `ApproveSkillProposal`, `ArchiveSkill`, `ListDiscoveredSkills`, `ListManagedSkills`, `ListSkillProposals`, `RejectSkillProposal`, `RestoreSkill` | complete | Discovered, managed, and proposal Readers; archive/restore; confirmed approve/reject bound to workspace + scope + name + full immutable revision; same-name revisions remain distinct; `skills.changed` refetches only an open Skill projection. |
| MCP | `CreateMCPAuthorizationAttempt`, `CreateMCPServer`, `DeleteMCPServer`, `GetMCPAuthorizationAttempt`, `ListMCPServers`, `ListMCPTools`, `ReconnectMCPServer`, `TestMCPServer`, `UpdateMCPServer` | complete | `/mcp`, `/mcp-tools`, `/mcp-create`, `/mcp-edit`, `/mcp-probe`, `/mcp-delete`, `/mcp-reconnect`, and `/mcp-auth` expose secret-safe connection management, schemas, health probes, reconnect, and the complete browser authorization lifecycle; `mcp.changed` refetches only the open server/tool projection. |
| Schedules | `CreateSchedule`, `DeleteSchedule`, `ListSchedules`, `RunScheduleNow`, `UpdateSchedule` | complete | `/schedules`, `/schedule-create`, `/schedule-edit`, `/schedule-enable`, `/schedule-disable`, `/schedule-run`, and `/schedule-delete` expose cursor-complete reads, revision-guarded edits, lifecycle, immediate firing handles, destructive confirmation, and `schedules.changed` refetch. |
| Tools | `InvokeTool`, `ListTools` | complete | `/tools` and `/tool-invoke` expose only the runtime's direct read-only diagnostics, retain schemas and arguments as owned JSON, reject non-safe/duplicate/unpageable catalogs, resolve exact or unique names, confine every invocation to the admitted workspace, and render the canonical JSON result. Normal agent tool execution remains runtime-owned. |
| Codebase | `GetCodebaseStatus`, `ReindexCodebase`, `SearchCodebase` | complete | `/codebase`, `/codebase-search`, and confirmed `/codebase-reindex` expose the closed index lifecycle, parsed index time, counts/truncation/model, scored source spans, background operation identity, and semantic search independently from workspace grep. |
| Agent documents and recipes | `ListAgentDocs`, `ListRecipes` | complete | `/agent-docs` and `/recipes` expose source/scope provenance. `/recipe` resolves an exact or unique name, expands `$ARGUMENTS` and `$1..$9` with the documented boundary semantics, and opens a resize-safe multiline review whose save enters the unified send-or-queue path. |
| Hooks | `ListHooks`, `SetHookTrust` | complete | `/hooks` exposes exact executable/declarative actions, lifecycle event, matcher, timeout, source, scope, active state, trust root, and trust state. Trust and revocation re-read the current catalog, require resize-safe confirmation, bind the mutation to the reported project root, and refetch afterward. |
| Feedback | `CreateFeedback` | complete | `/feedback` validates a positive/negative rating plus optional note and targets the latest durable assistant item when available, otherwise the current run/session. The write uses normal command idempotency and remains outside the conversation aggregate. |

## Batch order

1. Workspace inspection plus `files.changed` invalidation — complete.
2. Session rollback/portability, run steering, and session/run/state/interrupt
   invalidations — complete.
3. Goals, usage, model roles, and providers — complete.
4. Skills and MCP, including terminal authorization — complete.
5. Schedules, agent memory, and knowledge — complete.
6. Codebase, direct diagnostic tools, agent documents/recipes, hooks, and
   feedback — complete.

Each batch must pass package tests, the race detector, architecture guards,
static analysis, native and cross-platform builds, and a real embedded-runtime
smoke test before its status advances.
