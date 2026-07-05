import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import { durationText } from "@/plugins/builtin/agent/public/runDigest";

type ApprovalDigest = RunDigest["approvals"][number];
type ChangedFile = RunDigest["changedFiles"][number];
type CommandDigest = RunDigest["commands"][number];

export interface RunSummaryBadge {
  labelKey: string;
  className: string;
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
  elapsedSuffix?: string;
}

const STATUS_BADGE_BY_STATUS: Record<RunDigest["status"], RunSummaryBadge> = {
  ok: {
    labelKey: "runSummary.status.done",
    className: "bg-success/12 text-success",
  },
  err: {
    labelKey: "runSummary.status.errored",
    className: "bg-negative/12 text-negative",
  },
  running: {
    labelKey: "runSummary.status.running",
    className: "bg-accent/12 text-accent",
  },
  unknown: {
    labelKey: "runSummary.status.unknown",
    className: "bg-surface-2 text-fg-muted",
  },
};

const APPROVAL_BADGE_BY_DECISION: Record<
  NonNullable<ApprovalDigest["decision"]> | "pending",
  RunSummaryBadge
> = {
  approved: {
    labelKey: "runSummary.approval.approved",
    className: "text-success",
  },
  declined: {
    labelKey: "runSummary.approval.declined",
    className: "text-warning",
  },
  pending: {
    labelKey: "runSummary.approval.pending",
    className: "text-fg-faint",
  },
};

export function runSummaryViewModel(
  digest: RunDigest,
  options: RunSummaryViewModelOptions = {},
): RunSummaryViewModel {
  return {
    subtext: runSummarySubtext(digest, options),
    statusBadge: STATUS_BADGE_BY_STATUS[digest.status],
    changedFiles: section(digest.changedFiles),
    readFiles: section(digest.readFiles),
    commands: section(digest.commands),
    approvals: section(digest.approvals),
    errors: section(digest.errors),
  };
}

export function runSummarySubtext(
  digest: Pick<RunDigest, "runId" | "startedAt" | "endedAt">,
  { now = Date.now(), elapsedSuffix = "" }: RunSummaryViewModelOptions = {},
): string {
  const runLabel = `run ${digest.runId ?? "—"}`;
  if (digest.startedAt == null) {
    return runLabel;
  }

  const ended = digest.endedAt != null;
  const end = digest.endedAt ?? now;
  return `${runLabel} · ${durationText(digest.startedAt, end)}${ended ? "" : elapsedSuffix}`;
}

export function runSummaryCommandClassName(status: CommandDigest["status"]): string {
  return status === "err" ? "text-negative" : "text-fg-muted";
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
