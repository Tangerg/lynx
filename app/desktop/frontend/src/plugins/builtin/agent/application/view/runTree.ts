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

export function selectCurrentRootMessages(view: AgentSessionView): Message[] {
  const runId = selectCurrentRootRun(view)?.id;
  if (!runId) {
    const local = view.messages.filter((message) => message.runId === null);
    return local.length > 0 ? local : EMPTY_MESSAGES;
  }
  return view.messages.filter((message) => message.runId === runId || message.runId === null);
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
