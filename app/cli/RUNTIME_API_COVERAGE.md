# Runtime API consumption ledger

This is the progress contract for consuming the public in-process runtime API
from `app/cli`. It is generated from the exported methods of
`app/runtime/embedded.Runtime`, then reviewed by bounded context. Public runtime
protocol DTOs remain confined to `internal/runtimeembedded`; every other package
uses a CLI-owned domain model and a consumer-owned port.

Baseline date: 2026-08-12
Runtime API inventory: 87 exported methods
Production API consumption: 64 methods
Queued API consumption: 23 methods

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
| Runtime invalidations | `SubscribeRuntime` | partial | One negotiated attach-first subscription consumes `files.changed`, `skills.changed`, `mcp.changed`, `sessions.changed`, `runs.changed`, `state.changed`, `goals.changed`, and `interrupts.changed`. File events re-read workspace changes; session/run/state/interrupt events re-read the authoritative session without taking ownership from an active stream; Skill, MCP, and goal events refresh only their open projections. Reconnects and sequence gaps resync every subscribed topic. Add schedules when its query surface lands. |
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
| Agent memory | `AddAgentMemory`, `DeleteAgentMemory`, `ListAgentMemory`, `ReviewAgentMemory`, `UpdateAgentMemory` | queued | Add a reviewable memory manager with explicit mutation confirmation. |
| Knowledge | `GetKnowledge`, `ListKnowledge`, `UpdateKnowledge` | queued | Add provenance-aware knowledge browser/editor and invalidation policy when exposed. |
| Skills | `ApproveSkillProposal`, `ArchiveSkill`, `ListDiscoveredSkills`, `ListManagedSkills`, `ListSkillProposals`, `RejectSkillProposal`, `RestoreSkill` | complete | Discovered, managed, and proposal Readers; archive/restore; confirmed approve/reject bound to workspace + scope + name + full immutable revision; same-name revisions remain distinct; `skills.changed` refetches only an open Skill projection. |
| MCP | `CreateMCPAuthorizationAttempt`, `CreateMCPServer`, `DeleteMCPServer`, `GetMCPAuthorizationAttempt`, `ListMCPServers`, `ListMCPTools`, `ReconnectMCPServer`, `TestMCPServer`, `UpdateMCPServer` | complete | `/mcp`, `/mcp-tools`, `/mcp-create`, `/mcp-edit`, `/mcp-probe`, `/mcp-delete`, `/mcp-reconnect`, and `/mcp-auth` expose secret-safe connection management, schemas, health probes, reconnect, and the complete browser authorization lifecycle; `mcp.changed` refetches only the open server/tool projection. |
| Schedules | `CreateSchedule`, `DeleteSchedule`, `ListSchedules`, `RunScheduleNow`, `UpdateSchedule` | queued | Add schedule catalog/editor/run-now history and `schedules.changed` refetch. |
| Tools | `InvokeTool`, `ListTools` | queued | Add an inspect/invoke surface for diagnostics; normal agent tool execution remains runtime-owned. |
| Codebase | `GetCodebaseStatus`, `ReindexCodebase`, `SearchCodebase` | queued | Add indexing status, explicit reindex, and semantic search distinct from workspace grep. |
| Agent documents and recipes | `ListAgentDocs`, `ListRecipes` | queued | Add discoverable authoring/context catalogs and prompt insertion. |
| Hooks | `ListHooks`, `SetHookTrust` | queued | Add hook inventory and an explicit trust decision surface. |
| Feedback | `CreateFeedback` | queued | Add scoped run/session feedback after outcome completion. |

## Batch order

1. Workspace inspection plus `files.changed` invalidation — complete.
2. Session rollback/portability, run steering, and session/run/state/interrupt
   invalidations — complete.
3. Goals, usage, model roles, and providers — complete.
4. Skills and MCP, including terminal authorization — complete.
5. Schedules, memory, knowledge, codebase, tools, hooks, docs/recipes, and
   feedback.

Each batch must pass package tests, the race detector, architecture guards,
static analysis, native and cross-platform builds, and a real embedded-runtime
smoke test before its status advances.
