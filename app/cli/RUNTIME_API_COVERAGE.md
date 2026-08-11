# Runtime API consumption ledger

This is the progress contract for consuming the public in-process runtime API
from `app/cli`. It is generated from the exported methods of
`app/runtime/embedded.Runtime`, then reviewed by bounded context. Public runtime
protocol DTOs remain confined to `internal/runtimeembedded`; every other package
uses a CLI-owned domain model and a consumer-owned port.

Baseline date: 2026-08-12
Runtime API inventory: 87 exported methods

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
| Runtime invalidations | `SubscribeRuntime` | partial | `files.changed` is negotiated, watched, sequence-checked, re-fetched, reconnected, and projected into the header/reader. Add consumers for skills, MCP, schedules, sessions, runs, state, goals, and interrupts as their views land. |
| Sessions | `CreateSession`, `DeleteSession`, `ForkSession`, `GetSession`, `ListSessions`, `UpdateSession` | complete | Interactive session center, switching, creation, rename, favorite, fork, delete, and cold snapshot recovery. |
| Session portability and rewind | `ExportSession`, `ImportSession`, `RollbackSession` | queued | The existing local transcript exporter is not a substitute for runtime portability. Add native export/import resources and authoritative rollback UI. |
| Runs | `CancelRun`, `GetRun`, `ListRuns`, `ResumeRun`, `StartRun`, `SubscribeRun` | complete | Core streaming, reconnect/replay, recovery, cancellation, HITL resume, and timeline. `GetRun`/`ListRuns` are consumed by the cold session projection. |
| Run steering | `SteerRun` | queued | Add a distinct steer-now interaction; do not disguise queued follow-ups as steering. |
| Run resources | `GetPlan`, `ListInterrupts`, `ListItems` | complete | Folded into the authoritative cold snapshot and recovery/HITL projections. |
| Models | `ListModels` | complete | Provider-qualified model picker and run options. |
| Model roles | `GetEmbeddingRole`, `GetUtilityRole`, `SetEmbeddingRole`, `SetUtilityRole` | queued | Add role inspection and selection without mixing them into the primary run model picker. |
| Providers | `ListProviders`, `TestProvider`, `UpdateProvider` | queued | Add provider status, configuration editing, secret-safe diagnostics, and test feedback. |
| Approvals | `ForgetApprovalRule`, `GetApprovalMode`, `ListApprovalRules`, `SetApprovalMode` | complete | Approval mode picker, remembered-rule catalog/deletion, and HITL decisions. |
| Workspace catalog | `ListWorkspaces`, `ResolveWorkspace` | complete | Runtime-known workspace inspector and picker; explicit workspace changes resolve through the authoritative runtime service. |
| Workspace inspection | `GetWorkspaceDiff`, `GetWorkspaceFileHead`, `ListWorkspaceFileChanges`, `ListWorkspaceFiles`, `ReadWorkspaceFile`, `SearchWorkspaceFiles` | complete | `/diff`, `/preview`, `/changes`, `/browse`, `/read`, and `/grep`, all rendered in the searchable full reader. |
| Usage | `GetSessionUsage`, `GetUsageSummary` | queued | Add session and global usage/cost views; retain stream usage as the active-run source. |
| Goals | `GetGoal`, `ResumeGoal`, `StartGoal`, `StopGoal` | queued | Add goal lifecycle, budget display, resume/stop, and `goals.changed` refetch. |
| Agent memory | `AddAgentMemory`, `DeleteAgentMemory`, `ListAgentMemory`, `ReviewAgentMemory`, `UpdateAgentMemory` | queued | Add a reviewable memory manager with explicit mutation confirmation. |
| Knowledge | `GetKnowledge`, `ListKnowledge`, `UpdateKnowledge` | queued | Add provenance-aware knowledge browser/editor and invalidation policy when exposed. |
| Skills | `ApproveSkillProposal`, `ArchiveSkill`, `ListDiscoveredSkills`, `ListManagedSkills`, `ListSkillProposals`, `RejectSkillProposal`, `RestoreSkill` | queued | Add discovered/managed/proposal views, proposal review workflow, archive/restore, and `skills.changed` refetch. |
| MCP | `CreateMCPAuthorizationAttempt`, `CreateMCPServer`, `DeleteMCPServer`, `GetMCPAuthorizationAttempt`, `ListMCPServers`, `ListMCPTools`, `ReconnectMCPServer`, `TestMCPServer`, `UpdateMCPServer` | queued | Add server/tool manager, terminal authorization lifecycle, health/test/reconnect, and `mcp.changed` refetch. |
| Schedules | `CreateSchedule`, `DeleteSchedule`, `ListSchedules`, `RunScheduleNow`, `UpdateSchedule` | queued | Add schedule catalog/editor/run-now history and `schedules.changed` refetch. |
| Tools | `InvokeTool`, `ListTools` | queued | Add an inspect/invoke surface for diagnostics; normal agent tool execution remains runtime-owned. |
| Codebase | `GetCodebaseStatus`, `ReindexCodebase`, `SearchCodebase` | queued | Add indexing status, explicit reindex, and semantic search distinct from workspace grep. |
| Agent documents and recipes | `ListAgentDocs`, `ListRecipes` | queued | Add discoverable authoring/context catalogs and prompt insertion. |
| Hooks | `ListHooks`, `SetHookTrust` | queued | Add hook inventory and an explicit trust decision surface. |
| Feedback | `CreateFeedback` | queued | Add scoped run/session feedback after outcome completion. |

## Batch order

1. Workspace inspection plus `files.changed` invalidation — implemented in the
   current batch.
2. Session rollback/portability, run steering, and session/run/state/interrupt
   invalidations.
3. Goals, usage, model roles, and providers.
4. Skills and MCP, including terminal authorization.
5. Schedules, memory, knowledge, codebase, tools, hooks, docs/recipes, and
   feedback.

Each batch must pass package tests, the race detector, architecture guards,
static analysis, native and cross-platform builds, and a real embedded-runtime
smoke test before its status advances.
