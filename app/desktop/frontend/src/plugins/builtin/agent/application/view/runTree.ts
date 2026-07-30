import type {
  AgentProblem,
  AgentRunView,
  AgentSessionView,
  Message,
  PlanItem,
  RunUsage,
} from "@/plugins/sdk/types/agentSessionView";

const EMPTY_PLAN: PlanItem[] = [];
const EMPTY_MESSAGES: Message[] = [];
const EMPTY_USAGE: RunUsage = {
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
};

export interface AgentRunTreeNode {
  run: AgentRunView;
  children: AgentRunTreeNode[];
}

export interface DelegatedRunNarrative {
  run: AgentRunView;
  messages: Message[];
  plan: PlanItem[];
}

export type DelegatedRunNarrativesByItemId = Record<string, DelegatedRunNarrative[]>;

export type AgentRootAttention =
  { status: "idle"; runId: null } | { status: AgentRunView["status"]; runId: string };

function compareRuns(left: AgentRunView, right: AgentRunView): number {
  const byCreatedAt = left.createdAt.localeCompare(right.createdAt);
  return byCreatedAt || left.id.localeCompare(right.id);
}

export function selectRootRuns(view: AgentSessionView): AgentRunView[] {
  return Object.values(view.runsById)
    .filter((run) => run.parentRunId === null)
    .sort(compareRuns);
}

export function selectCurrentRootRun(view: AgentSessionView): AgentRunView | null {
  let latest: AgentRunView | null = null;
  let latestOpen: AgentRunView | null = null;
  for (const run of Object.values(view.runsById)) {
    if (run.parentRunId !== null) continue;
    if (!latest || compareRuns(latest, run) < 0) latest = run;
    if (run.status !== "finished" && (!latestOpen || compareRuns(latestOpen, run) < 0)) {
      latestOpen = run;
    }
  }
  return latestOpen ?? latest;
}

export function selectCurrentRootAttention(view: AgentSessionView): AgentRootAttention {
  const root = selectCurrentRootRun(view);
  return root ? { status: root.status, runId: root.id } : { status: "idle", runId: null };
}

export function selectRun(
  view: AgentSessionView,
  runId: string | null | undefined,
): AgentRunView | null {
  return runId ? (view.runsById[runId] ?? null) : null;
}

export function selectRunPlan(
  view: AgentSessionView,
  runId: string | null | undefined,
): PlanItem[] {
  return runId ? (view.plansByRunId[runId] ?? EMPTY_PLAN) : EMPTY_PLAN;
}

export function selectCurrentRootPlan(view: AgentSessionView): PlanItem[] {
  return selectRunPlan(view, selectCurrentRootRun(view)?.id);
}

/**
 * The main Session narrative: optimistic local messages plus material owned by
 * every root Run, in projection order. Descendant material is selected
 * separately under its durable parent Item anchor.
 */
export function selectRootNarrativeMessages(view: AgentSessionView): Message[] {
  const rootRunIds = new Set(selectRootRuns(view).map((run) => run.id));
  const messages = view.messages.filter(
    (message) => message.runId === null || rootRunIds.has(message.runId),
  );
  return messages.length > 0 ? messages : EMPTY_MESSAGES;
}

export function selectDelegatedRunNarratives(
  view: AgentSessionView,
): DelegatedRunNarrativesByItemId {
  const byItemId: DelegatedRunNarrativesByItemId = {};
  for (const run of Object.values(view.runsById).sort(compareRuns)) {
    if (run.parentRunId === null || run.spawnedByItemId === null) continue;
    const narrative: DelegatedRunNarrative = {
      run,
      messages: view.messages.filter((message) => message.runId === run.id),
      plan: selectRunPlan(view, run.id),
    };
    (byItemId[run.spawnedByItemId] ??= []).push(narrative);
  }
  return byItemId;
}

/**
 * A derived forest over normalized Run facts. Snapshot hydration validates
 * connected lineage; retaining a missing-parent node as a root keeps malformed
 * live material auditable instead of silently hiding it.
 */
export function selectRunTree(view: AgentSessionView): AgentRunTreeNode[] {
  const runs = Object.values(view.runsById).sort(compareRuns);
  const byRunId = new Map<string, AgentRunTreeNode>();
  for (const run of runs) byRunId.set(run.id, { run, children: [] });

  const roots: AgentRunTreeNode[] = [];
  for (const run of runs) {
    const node = byRunId.get(run.id)!;
    const parent = run.parentRunId ? byRunId.get(run.parentRunId) : undefined;
    if (parent && parent !== node) parent.children.push(node);
    else roots.push(node);
  }
  return roots;
}

export function selectRunUsage(run: AgentRunView | null): RunUsage {
  return run?.progress?.usage ?? run?.metrics.usage ?? EMPTY_USAGE;
}

export function selectRunProblem(run: AgentRunView | null): AgentProblem | null {
  return run?.outcome?.type === "error" ? run.outcome.error : null;
}

export function selectVisibleProblem(view: AgentSessionView): AgentProblem | null {
  if (view.commandError) return view.commandError;
  const run = selectCurrentRootRun(view);
  return run?.id === view.dismissedProblemRunId ? null : selectRunProblem(run);
}
