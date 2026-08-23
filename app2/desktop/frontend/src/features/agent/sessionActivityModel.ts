import type {
  Interrupt,
  Item,
  PendingInterruptSet,
  RunRef,
  Usage,
} from "@lyra/runtime-contract";

import type { LiveToolOutput } from "./agentSessionTypes";
import {
  isRecord,
  presentTool,
  stringArgument,
  type ToolPresentation,
} from "./toolPresentation";

const maxSummaryEntries = 100;

export interface TimelineEntry {
  id: string;
  kind:
    | "runStarted"
    | "tool"
    | "interrupt"
    | "runSettled"
    | "runWaiting";
  run: RunRef;
  depth: number;
  timestamp?: string;
  item?: Item;
  interrupt?: Interrupt;
  tool?: ToolPresentation;
}

export interface TimelineGroup {
  root: RunRef;
  runs: RunRef[];
  entries: TimelineEntry[];
  integrity: string[];
}

export interface TerminalCommand {
  id: string;
  run: RunRef;
  item: Item;
  command: string;
  startedAt?: string;
  finishedAt?: string;
  stdout: string;
  stderr: string;
  liveOutput?: LiveToolOutput;
  exitCode?: number;
  killed: boolean;
}

export interface SummaryChange {
  path: string;
  action: "created" | "deleted" | "moved" | "modified";
}

export interface SummaryApproval {
  tool: string;
  subject?: string;
  decision: string;
}

export interface SummaryError {
  source: string;
  detail: string;
}

export interface SessionRunSummary {
  root: RunRef;
  runs: RunRef[];
  status: string;
  startedAt?: string;
  finishedAt?: string;
  activeDurationMillis: number;
  steps: number;
  usage: Usage;
  changes: SummaryChange[];
  readFiles: string[];
  commands: TerminalCommand[];
  approvals: SummaryApproval[];
  errors: SummaryError[];
  omitted: {
    changes: number;
    readFiles: number;
    commands: number;
    approvals: number;
    errors: number;
  };
}

export function buildTimeline(
  runs: RunRef[],
  items: Item[],
  interruptSets: PendingInterruptSet[] = [],
): TimelineGroup[] {
  const runById = new Map(runs.map((run) => [run.id, run]));
  const groups = new Map<string, { runs: RunRef[]; integrity: Set<string> }>();
  for (const run of runs) {
    const resolution = resolveRoot(run, runById);
    const group = groups.get(resolution.root.id) ?? {
      runs: [],
      integrity: new Set<string>(),
    };
    group.runs.push(run);
    if (resolution.integrity) group.integrity.add(resolution.integrity);
    groups.set(resolution.root.id, group);
  }

  const itemsByRunId = new Map<string, Item[]>();
  for (const item of items) {
    const values = itemsByRunId.get(item.runId) ?? [];
    values.push(item);
    itemsByRunId.set(item.runId, values);
  }
  const interruptsByRunId = new Map<
    string,
    { interrupt: Interrupt; createdAt: string }[]
  >();
  for (const set of interruptSets) {
    for (const interrupt of set.interrupts) {
      const values = interruptsByRunId.get(interrupt.runId) ?? [];
      values.push({ interrupt, createdAt: set.createdAt });
      interruptsByRunId.set(interrupt.runId, values);
    }
  }

  return [...groups.entries()]
    .map(([rootId, group]) => {
      const root = runById.get(rootId) ?? group.runs[0]!;
      const runIds = new Set(group.runs.map((run) => run.id));
      const entries = group.runs.flatMap((run) => {
        const depth = runDepth(run, runById, runIds);
        const material = itemsByRunId.get(run.id) ?? [];
        const pendingInterrupts = interruptsByRunId.get(run.id) ?? [];
        const startedAt = run.createdAt ?? earliestTimestamp(material);
        const values: TimelineEntry[] = [
          {
            id: `${run.id}:started`,
            kind: "runStarted",
            run,
            depth,
            ...(startedAt ? { timestamp: startedAt } : {}),
          },
        ];
        for (const item of material) {
          if (item.type !== "toolCall") continue;
          values.push({
            id: item.id,
            kind: "tool",
            run,
            depth,
            item,
            tool: presentTool(item.tool?.name ?? "", item.tool?.arguments ?? {}),
            ...((item.startedAt ?? item.createdAt)
              ? { timestamp: item.startedAt ?? item.createdAt }
              : {}),
          });
        }
        for (const pending of pendingInterrupts) {
          const tool = pending.interrupt.payload?.tool;
          values.push({
            id: `${pending.interrupt.itemId}:interrupt`,
            kind: "interrupt",
            run,
            depth,
            interrupt: pending.interrupt,
            ...(tool
              ? { tool: presentTool(tool.name, tool.arguments) }
              : {}),
            timestamp: pending.createdAt,
          });
        }
        if (run.status === "waiting") {
          const waitingAt = latestOf(
            latestTimestamp(material, run.createdAt),
            ...pendingInterrupts.map((pending) => pending.createdAt),
          );
          values.push({
            id: `${run.id}:waiting`,
            kind: "runWaiting",
            run,
            depth,
            ...(waitingAt ? { timestamp: waitingAt } : {}),
          });
        } else if (run.finishedAt || run.status === "finished") {
          values.push({
            id: `${run.id}:settled`,
            kind: "runSettled",
            run,
            depth,
            ...(run.finishedAt ? { timestamp: run.finishedAt } : {}),
          });
        }
        return values;
      });
      return {
        root,
        runs: group.runs.toSorted(compareOccurred),
        entries: entries.toSorted(compareTimelineEntry),
        integrity: [...group.integrity],
      };
    })
    .toSorted((left, right) => compareOccurred(right.root, left.root));
}

export function buildTerminalCommands(
  runs: RunRef[],
  items: Item[],
  liveOutputs: Record<string, LiveToolOutput>,
): TerminalCommand[] {
  const runById = new Map(runs.map((run) => [run.id, run]));
  return items
    .flatMap((item) => {
      const run = runById.get(item.runId);
      if (
        run === undefined ||
        item.type !== "toolCall" ||
        item.tool?.name !== "shell"
      ) {
        return [];
      }
      const result = isRecord(item.tool.result) ? item.tool.result : {};
      const liveOutput = liveOutputs[item.id];
      return [
        {
          id: item.id,
          run,
          item,
          command:
            stringArgument(item.tool.arguments, "command") ??
            "Command unavailable",
          ...(item.startedAt ? { startedAt: item.startedAt } : {}),
          ...(item.finishedAt ? { finishedAt: item.finishedAt } : {}),
          stdout: typeof result.stdout === "string" ? result.stdout : "",
          stderr: typeof result.stderr === "string" ? result.stderr : "",
          ...(liveOutput === undefined ? {} : { liveOutput }),
          ...(typeof result.exit_code === "number"
            ? { exitCode: result.exit_code }
            : {}),
          killed: result.killed === true,
        },
      ];
    })
    .toSorted((left, right) => compareOccurred(left.item, right.item));
}

export function buildLatestRunSummary(
  runs: RunRef[],
  items: Item[],
  liveOutputs: Record<string, LiveToolOutput>,
): SessionRunSummary | undefined {
  const latest = buildTimeline(runs, items)[0];
  if (latest === undefined) return undefined;
  const runIds = new Set(latest.runs.map((run) => run.id));
  const material = items.filter((item) => runIds.has(item.runId));
  const allChanges = uniqueChanges(material.flatMap(changesFromItem));
  const allReads = uniqueStrings(material.flatMap(readsFromItem));
  const allCommands = buildTerminalCommands(latest.runs, material, liveOutputs);
  const allApprovals = material.flatMap(approvalFromItem);
  const allErrors = [
    ...latest.runs.flatMap(errorFromRun),
    ...material.flatMap(errorFromItem),
  ];
  return {
    root: latest.root,
    runs: latest.runs,
    status: runStatus(latest.root),
    ...(latest.root.createdAt ? { startedAt: latest.root.createdAt } : {}),
    ...(latest.root.finishedAt ? { finishedAt: latest.root.finishedAt } : {}),
    activeDurationMillis: latest.runs.reduce(
      (total, run) => total + run.metrics.activeDurationMillis,
      0,
    ),
    steps: latest.runs.reduce((total, run) => total + run.metrics.steps, 0),
    usage: latest.runs.reduce(mergeUsage, {}),
    changes: allChanges.slice(0, maxSummaryEntries),
    readFiles: allReads.slice(0, maxSummaryEntries),
    commands: allCommands.slice(0, maxSummaryEntries),
    approvals: allApprovals.slice(0, maxSummaryEntries),
    errors: allErrors.slice(0, maxSummaryEntries),
    omitted: {
      changes: omitted(allChanges),
      readFiles: omitted(allReads),
      commands: omitted(allCommands),
      approvals: omitted(allApprovals),
      errors: omitted(allErrors),
    },
  };
}

export function summaryAsText(summary: SessionRunSummary) {
  const lines = [
    `Run ${summary.root.id}`,
    `Status: ${summary.status}`,
    `Runs: ${summary.runs.length}`,
    `Steps: ${summary.steps}`,
    `Active time: ${formatDuration(summary.activeDurationMillis)}`,
  ];
  appendSection(
    lines,
    "Changed files",
    summary.changes.map((change) => `${change.action}: ${change.path}`),
    summary.omitted.changes,
  );
  appendSection(lines, "Read files", summary.readFiles, summary.omitted.readFiles);
  appendSection(
    lines,
    "Commands",
    summary.commands.map((command) => command.command),
    summary.omitted.commands,
  );
  appendSection(
    lines,
    "Approvals",
    summary.approvals.map(
      (approval) =>
        `${approval.decision}: ${approval.tool}${approval.subject ? ` — ${approval.subject}` : ""}`,
    ),
    summary.omitted.approvals,
  );
  appendSection(
    lines,
    "Errors",
    summary.errors.map((error) => `${error.source}: ${error.detail}`),
    summary.omitted.errors,
  );
  return lines.join("\n");
}

export function runStatus(run: RunRef) {
  if (run.status === "running") return "running";
  if (run.status === "waiting") return "waiting";
  return run.outcome?.type ?? run.status ?? "unknown";
}

function resolveRoot(run: RunRef, runById: Map<string, RunRef>) {
  if (run.parentRunId === undefined) return { root: run };
  const declared = run.rootRunId ? runById.get(run.rootRunId) : undefined;
  if (declared !== undefined && declared.parentRunId === undefined) {
    return { root: declared };
  }
  const seen = new Set([run.id]);
  let current = run;
  while (current.parentRunId !== undefined) {
    if (seen.has(current.parentRunId)) {
      return {
        root: run,
        integrity: `Run lineage contains a cycle at ${shortIdentity(current.parentRunId)}.`,
      };
    }
    seen.add(current.parentRunId);
    const parent = runById.get(current.parentRunId);
    if (parent === undefined) {
      return {
        root: run,
        integrity: `Parent run ${shortIdentity(current.parentRunId)} is unavailable.`,
      };
    }
    current = parent;
  }
  return { root: current };
}

function runDepth(
  run: RunRef,
  runById: Map<string, RunRef>,
  groupRunIds: Set<string>,
) {
  let depth = 0;
  let current = run;
  const seen = new Set([run.id]);
  while (current.parentRunId && groupRunIds.has(current.parentRunId)) {
    if (seen.has(current.parentRunId)) break;
    seen.add(current.parentRunId);
    const parent = runById.get(current.parentRunId);
    if (parent === undefined) break;
    depth += 1;
    current = parent;
  }
  return depth;
}

function latestTimestamp(items: Item[], fallback?: string) {
  const values = items.flatMap((item) => {
    const value = item.finishedAt ?? item.startedAt ?? item.createdAt;
    return value === undefined ? [] : [value];
  });
  if (fallback !== undefined) values.push(fallback);
  return values.toSorted().at(-1);
}

function earliestTimestamp(items: Item[]) {
  return items
    .flatMap((item) => {
      const value = item.startedAt ?? item.createdAt ?? item.finishedAt;
      return value === undefined ? [] : [value];
    })
    .toSorted()[0];
}

function latestOf(...values: (string | undefined)[]) {
  return values
    .filter((value): value is string => value !== undefined)
    .toSorted()
    .at(-1);
}

function compareTimelineEntry(left: TimelineEntry, right: TimelineEntry) {
  return compareTimestamp(left.timestamp, right.timestamp) ||
    timelineRank(left.kind) - timelineRank(right.kind) ||
    left.id.localeCompare(right.id);
}

function timelineRank(kind: TimelineEntry["kind"]) {
  if (kind === "runStarted") return 0;
  if (kind === "tool") return 1;
  if (kind === "interrupt") return 2;
  return 3;
}

function compareOccurred(
  left: Pick<Item, "id" | "createdAt" | "startedAt"> | RunRef,
  right: Pick<Item, "id" | "createdAt" | "startedAt"> | RunRef,
) {
  const leftTime =
    "startedAt" in left ? left.startedAt ?? left.createdAt : left.createdAt;
  const rightTime =
    "startedAt" in right ? right.startedAt ?? right.createdAt : right.createdAt;
  return compareTimestamp(leftTime, rightTime) || left.id.localeCompare(right.id);
}

function compareTimestamp(left: string | undefined, right: string | undefined) {
  if (left === right) return 0;
  if (left === undefined) return 1;
  if (right === undefined) return -1;
  return left.localeCompare(right);
}

function changesFromItem(item: Item): SummaryChange[] {
  if (item.type !== "toolCall" || item.tool === undefined) return [];
  if (item.tool.name === "edit" || item.tool.name === "write") {
    const path = stringArgument(item.tool.arguments, "path");
    return path ? [{ path, action: "modified" }] : [];
  }
  if (item.tool.name !== "apply_patch") return [];
  if (isRecord(item.tool.result) && Array.isArray(item.tool.result.files)) {
    return item.tool.result.files.flatMap((value) => {
      if (!isRecord(value) || typeof value.path !== "string") return [];
      return [{
        path: value.path,
        action: value.deleted === true
          ? "deleted"
          : value.created === true
            ? "created"
            : typeof value.moved_from === "string"
              ? "moved"
              : "modified",
      } satisfies SummaryChange];
    });
  }
  const patch = stringArgument(item.tool.arguments, "patch") ?? "";
  return [...patch.matchAll(/^\*\*\* (Update|Add|Delete) File: (.+)$/gm)].map(
    (match) => ({
      path: match[2] ?? "Unknown path",
      action:
        match[1] === "Add"
          ? "created"
          : match[1] === "Delete"
            ? "deleted"
            : "modified",
    }),
  );
}

function readsFromItem(item: Item) {
  if (item.type !== "toolCall" || item.tool === undefined) return [];
  if (!new Set(["read", "grep", "glob", "lsp"]).has(item.tool.name)) return [];
  const path = stringArgument(item.tool.arguments, "path");
  return path ? [path] : [];
}

function approvalFromItem(item: Item): SummaryApproval[] {
  if (item.type !== "toolCall" || !item.approvalDecision) return [];
  const presentation = presentTool(
    item.tool?.name ?? "",
    item.tool?.arguments ?? {},
  );
  return [{
    tool: presentation.title,
    ...(presentation.subject ? { subject: presentation.subject } : {}),
    decision: item.approvalDecision,
  }];
}

function errorFromItem(item: Item): SummaryError[] {
  if (item.type !== "toolCall" || item.error === undefined) return [];
  return [{
    source: presentTool(item.tool?.name ?? "", item.tool?.arguments ?? {}).title,
    detail: item.error.detail ?? item.error.type,
  }];
}

function errorFromRun(run: RunRef): SummaryError[] {
  if (
    run.outcome === undefined ||
    run.outcome.type === "completed" ||
    run.outcome.type === "canceled"
  ) {
    return [];
  }
  return [{
    source: `Run ${shortIdentity(run.id)}`,
    detail: run.outcome.error?.detail ?? run.outcome.detail ?? run.outcome.type,
  }];
}

function mergeUsage(total: Usage, run: RunRef): Usage {
  const usage = run.metrics.usage;
  if (usage === undefined) return total;
  return {
    ...sumUsageField(total, usage, "inputTokens"),
    ...sumUsageField(total, usage, "outputTokens"),
    ...sumUsageField(total, usage, "cacheReadTokens"),
    ...sumUsageField(total, usage, "cacheWriteTokens"),
    ...sumUsageField(total, usage, "reasoningTokens"),
    ...sumUsageField(total, usage, "costUsd"),
  };
}

function sumUsageField(
  total: Usage,
  addition: Usage,
  field: keyof Pick<
    Usage,
    | "inputTokens"
    | "outputTokens"
    | "cacheReadTokens"
    | "cacheWriteTokens"
    | "reasoningTokens"
    | "costUsd"
  >,
) {
  const left = total[field];
  const right = addition[field];
  return left === undefined && right === undefined
    ? {}
    : { [field]: (left ?? 0) + (right ?? 0) };
}

function uniqueChanges(values: SummaryChange[]) {
  const byPath = new Map<string, SummaryChange>();
  for (const value of values) byPath.set(value.path, value);
  return [...byPath.values()];
}

function uniqueStrings(values: string[]) {
  return [...new Set(values)];
}

function omitted(values: unknown[]) {
  return Math.max(0, values.length - maxSummaryEntries);
}

function appendSection(
  lines: string[],
  title: string,
  values: string[],
  omittedCount: number,
) {
  if (values.length === 0 && omittedCount === 0) return;
  lines.push("", `${title}:`, ...values.map((value) => `- ${value}`));
  if (omittedCount > 0) lines.push(`- … ${omittedCount} more`);
}

function formatDuration(milliseconds: number) {
  const seconds = Math.round(milliseconds / 1_000);
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

function shortIdentity(identity: string) {
  return identity.length <= 16
    ? identity
    : `${identity.slice(0, 8)}…${identity.slice(-5)}`;
}
