import type { Translate } from "@/lib/i18n";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import {
  agentRunDetail,
  agentRunPresentationState,
  agentRunStepCount,
  type AgentRunPresentationState,
} from "@/plugins/builtin/agent/public/runPresentation";

type DotTone = "idle" | "running" | "waiting" | "ok" | "err";

export interface DelegatedRunCardModel {
  label: string;
  status: AgentRunPresentationState;
  statusLabel: string;
  dotTone: DotTone;
  detail: string | null;
  stepsLabel: string;
  autoExpanded: boolean;
  cancelable: boolean;
}

const STATUS_VIEW: Record<AgentRunPresentationState, { labelKey: string; dotTone: DotTone }> = {
  running: { labelKey: "agent.runTree.status.running", dotTone: "running" },
  waiting: { labelKey: "agent.runTree.status.waiting", dotTone: "waiting" },
  finished: { labelKey: "agent.runTree.status.finished", dotTone: "ok" },
  error: { labelKey: "agent.runTree.status.error", dotTone: "err" },
  canceled: { labelKey: "agent.runTree.status.canceled", dotTone: "idle" },
  limit: { labelKey: "agent.runTree.status.limit", dotTone: "waiting" },
};

export function delegatedRunCardModel(
  t: Translate,
  run: AgentRunView,
  ordinal: number,
  siblingCount: number,
): DelegatedRunCardModel {
  const status = agentRunPresentationState(run);
  const statusView = STATUS_VIEW[status];
  return {
    label:
      siblingCount === 1
        ? t("agent.runTree.delegated.one")
        : t("agent.runTree.delegated.many", { index: ordinal, count: siblingCount }),
    status,
    statusLabel: t(statusView.labelKey),
    dotTone: statusView.dotTone,
    detail: agentRunDetail(run),
    stepsLabel: t("agent.steps", { count: agentRunStepCount(run) }),
    // Exempt from the answer-supersedes-work rule on purpose: a delegated run that is
    // waiting has an interrupt somebody has to act on, and folding away a request for
    // a decision because the parent started talking is how a turn deadlocks in
    // silence. Every other status auto-collapses already.
    autoExpanded: status === "waiting",
    cancelable: run.status !== "finished",
  };
}
