import type { Translate } from "@/lib/i18n";
import type { Tone } from "@/lib/tone";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import { durationText } from "@/plugins/builtin/agent/public/runDigest";

type ApprovalDigest = RunDigest["approvals"][number];
type ChangedFile = RunDigest["changedFiles"][number];
type CommandDigest = RunDigest["commands"][number];

// A label plus the tone it reads in. Not a className: the rendering layer owns
// which fill or ink expresses that semantic tone.
export interface RunSummaryBadge {
  labelKey: string;
  tone: Tone;
}

export interface RunSummarySection<T> {
  items: readonly T[];
  count: number;
}

export interface RunSummaryViewModel {
  subtext: string;
  statusBadge: RunSummaryBadge;
  changedFiles: RunSummarySection<ChangedFile>;
  readFiles: RunSummarySection<string>;
  commands: RunSummarySection<CommandDigest>;
  approvals: RunSummarySection<ApprovalDigest>;
  errors: RunSummarySection<string>;
}

export interface RunSummaryViewModelOptions {
  now?: number;
}

const STATUS_BADGE_BY_STATUS: Record<RunDigest["status"], RunSummaryBadge> = {
  ok: {
    labelKey: "runSummary.status.done",
    tone: "success",
  },
  err: {
    labelKey: "runSummary.status.errored",
    tone: "negative",
  },
  running: {
    labelKey: "runSummary.status.running",
    tone: "accent",
  },
  waiting: {
    labelKey: "runSummary.status.waiting",
    tone: "warning",
  },
  unknown: {
    labelKey: "runSummary.status.unknown",
    tone: "neutral",
  },
  canceled: {
    labelKey: "agent.runTree.status.canceled",
    tone: "neutral",
  },
  limit: {
    labelKey: "agent.runTree.status.limit",
    tone: "warning",
  },
};

const APPROVAL_BADGE_BY_DECISION: Record<
  NonNullable<ApprovalDigest["decision"]> | "pending",
  RunSummaryBadge
> = {
  approved: {
    labelKey: "runSummary.approval.approved",
    tone: "success",
  },
  declined: {
    labelKey: "runSummary.approval.declined",
    tone: "warning",
  },
  pending: {
    labelKey: "runSummary.approval.pending",
    tone: "neutral",
  },
};

export function runSummaryViewModel(
  t: Translate,
  digest: RunDigest,
  options: RunSummaryViewModelOptions = {},
): RunSummaryViewModel {
  return {
    subtext: runSummarySubtext(t, digest, options),
    statusBadge: STATUS_BADGE_BY_STATUS[digest.status],
    changedFiles: section(digest.changedFiles),
    readFiles: section(digest.readFiles),
    commands: section(digest.commands),
    approvals: section(digest.approvals),
    errors: section(digest.errors),
  };
}

export function runSummarySubtext(
  t: Translate,
  digest: Pick<RunDigest, "runId" | "startedAt" | "endedAt">,
  { now = Date.now() }: RunSummaryViewModelOptions = {},
): string {
  // The one worded fragment this view model still assembles. Every other label
  // here is a `labelKey` the view resolves; a run id needs the word woven in, so
  // the translator comes in instead — and the caller no longer has to hand the
  // "still running" suffix through options to keep it out of English.
  const runLabel = t("runSummary.subtext.run", { id: digest.runId ?? "—" });
  if (digest.startedAt == null) {
    return runLabel;
  }

  const ended = digest.endedAt != null;
  const end = digest.endedAt ?? now;
  const elapsed = ended ? "" : t("runSummary.elapsed");
  return `${runLabel} · ${durationText(t, digest.startedAt, end)}${elapsed}`;
}

export function runSummaryCommandTone(status: CommandDigest["status"]): Tone {
  return status === "err" ? "negative" : "neutral";
}

export function runSummaryApprovalBadge(decision: ApprovalDigest["decision"]): RunSummaryBadge {
  return APPROVAL_BADGE_BY_DECISION[decision ?? "pending"];
}

function section<T>(items: readonly T[]): RunSummarySection<T> {
  return {
    items,
    count: items.length,
  };
}
