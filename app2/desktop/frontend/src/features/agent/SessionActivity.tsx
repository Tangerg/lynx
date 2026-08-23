import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";

import type {
  Item,
  PendingInterruptSet,
  RunRef,
} from "@lyra/runtime-contract";

import type { LiveToolOutput, SessionActivityView } from "./agentSessionTypes";
import {
  buildLatestRunSummary,
  buildTerminalCommands,
  buildTimeline,
  runStatus,
  summaryAsText,
  type SessionRunSummary,
  type TerminalCommand,
  type TimelineEntry,
} from "./sessionActivityModel";
import { formatToolDuration, toolStatusLabel } from "./toolPresentation";

interface SessionActivityProps {
  view: SessionActivityView;
  runs: RunRef[];
  items: Item[];
  interrupts: PendingInterruptSet[];
  liveToolOutputs: Record<string, LiveToolOutput>;
  actionPending: boolean;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  onViewChange(view: SessionActivityView): void;
  onCancelRun(runId: string): Promise<void>;
  children: ReactNode;
}

const views: { id: SessionActivityView; label: string }[] = [
  { id: "overview", label: "Overview" },
  { id: "timeline", label: "Timeline" },
  { id: "terminal", label: "Terminal" },
  { id: "summary", label: "Summary" },
];

export function SessionActivity(props: SessionActivityProps) {
  return (
    <section className="session-activity" aria-label="Session activity">
      <nav className="session-view-switch" aria-label="Session views">
        {views.map((view) => (
          <button
            key={view.id}
            type="button"
            aria-current={props.view === view.id ? "page" : undefined}
            onClick={() => props.onViewChange(view.id)}
          >
            {view.label}
          </button>
        ))}
      </nav>
      {props.view === "overview" ? (
        <div className="session-overview-view">{props.children}</div>
      ) : props.view === "timeline" ? (
        <TimelineView {...props} />
      ) : props.view === "terminal" ? (
        <TerminalView
          runs={props.runs}
          items={props.items}
          liveToolOutputs={props.liveToolOutputs}
        />
      ) : (
        <SummaryView
          runs={props.runs}
          items={props.items}
          liveToolOutputs={props.liveToolOutputs}
        />
      )}
    </section>
  );
}

function TimelineView(props: SessionActivityProps) {
  const groups = useMemo(
    () => buildTimeline(props.runs, props.items, props.interrupts),
    [props.interrupts, props.items, props.runs],
  );
  if (groups.length === 0) {
    return (
      <ActivityEmpty
        title="No run timeline yet"
        detail="Run lifecycle and tool facts will appear here after Lyra starts work."
      />
    );
  }
  return (
    <div className="activity-scroll session-timeline-view">
      {groups.map((group, groupIndex) => (
        <section className="run-timeline-group" key={group.root.id}>
          <header>
            <div>
              <span className="eyebrow">
                {groupIndex === 0 ? "Latest run tree" : "Earlier run tree"}
              </span>
              <strong title={group.root.id}>
                {shortIdentity(group.root.id)}
              </strong>
            </div>
            <span className="run-state-chip" data-status={runStatus(group.root)}>
              {humanize(runStatus(group.root))}
            </span>
          </header>
          <p className="run-tree-facts">
            {group.runs.length} {group.runs.length === 1 ? "run" : "runs"}
            {group.root.model ? ` · ${group.root.model}` : ""}
            {group.root.provider ? ` · ${group.root.provider}` : ""}
          </p>
          {group.integrity.map((detail) => (
            <p className="activity-integrity" role="status" key={detail}>
              {detail}
            </p>
          ))}
          <ol className="run-timeline-list">
            {group.entries.map((entry) => (
              <TimelineRow
                key={entry.id}
                entry={entry}
                actionPending={props.actionPending}
                canceling={props.cancelingRunId === entry.run.id}
                cancelError={
                  props.cancelError?.runId === entry.run.id
                    ? props.cancelError.message
                    : undefined
                }
                onCancelRun={props.onCancelRun}
              />
            ))}
          </ol>
        </section>
      ))}
    </div>
  );
}

function TimelineRow(props: {
  entry: TimelineEntry;
  actionPending: boolean;
  canceling: boolean;
  cancelError?: string;
  onCancelRun(runId: string): Promise<void>;
}) {
  const entry = props.entry;
  const style = {
    "--activity-depth": Math.min(entry.depth, 6),
  } as CSSProperties;
  if (entry.kind === "interrupt" && entry.interrupt) {
    const approval = entry.interrupt.type === "approval";
    return (
      <li className="timeline-row timeline-interrupt-row" style={style}>
        <span className="timeline-mark" data-status="waiting" aria-hidden="true">
          !
        </span>
        <div className="timeline-row-body">
          <strong>{approval ? "Approval requested" : "Input requested"}</strong>
          {entry.tool?.subject ? <code>{entry.tool.subject}</code> : null}
          <span className="timeline-row-facts">
            {entry.tool ? <small>{entry.tool.title}</small> : null}
            <small>pending</small>
            <OccurredAt value={entry.timestamp} />
          </span>
          {entry.interrupt.payload?.reason ? (
            <p className="timeline-interrupt-reason">
              {entry.interrupt.payload.reason}
            </p>
          ) : null}
        </div>
      </li>
    );
  }
  if (entry.kind === "tool" && entry.item && entry.tool) {
    return (
      <li className="timeline-row timeline-tool-row" style={style}>
        <span
          className="timeline-mark"
          data-kind={entry.tool.kind}
          aria-hidden="true"
        >
          {entry.tool.glyph}
        </span>
        <div className="timeline-row-body">
          <strong>{entry.tool.title}</strong>
          {entry.tool.subject ? <code>{entry.tool.subject}</code> : null}
          <span className="timeline-row-facts">
            <small>{toolStatusLabel(entry.item)}</small>
            {entry.item.approvalDecision ? (
              <small>{entry.item.approvalDecision}</small>
            ) : null}
            {entry.item.durationMillis !== undefined ? (
              <small>{formatToolDuration(entry.item.durationMillis)}</small>
            ) : null}
            <OccurredAt value={entry.timestamp} />
          </span>
          {entry.item.error?.detail ? (
            <p role="alert">{entry.item.error.detail}</p>
          ) : null}
        </div>
      </li>
    );
  }
  const active =
    entry.kind === "runStarted" &&
    (entry.run.status === "running" || entry.run.status === "waiting");
  const title =
    entry.kind === "runStarted"
      ? entry.depth === 0
        ? "Run started"
        : "Delegated run started"
      : entry.kind === "runWaiting"
        ? "Run waiting for input"
        : `Run ${humanize(runStatus(entry.run))}`;
  const detail =
    entry.kind === "runSettled"
      ? entry.run.outcome?.error?.detail ?? entry.run.outcome?.detail
      : undefined;
  return (
    <li className="timeline-row timeline-run-row" style={style}>
      <span
        className="timeline-mark"
        data-status={runStatus(entry.run)}
        aria-hidden="true"
      />
      <div className="timeline-row-body">
        <strong>{title}</strong>
        <span className="timeline-run-identity" title={entry.run.id}>
          {shortIdentity(entry.run.id)}
        </span>
        <span className="timeline-row-facts">
          {entry.run.model ? <small>{entry.run.model}</small> : null}
          {entry.kind === "runSettled" ? (
            <small>{entry.run.metrics.steps} steps</small>
          ) : null}
          <OccurredAt value={entry.timestamp} />
        </span>
        {detail ? <p role="alert">{detail}</p> : null}
        {props.cancelError && entry.kind === "runStarted" ? (
          <p role="alert">{props.cancelError}</p>
        ) : null}
      </div>
      {active ? (
        <button
          className="quiet-action timeline-cancel"
          type="button"
          disabled={props.actionPending}
          onClick={() =>
            void props.onCancelRun(entry.run.id).catch(() => undefined)
          }
        >
          {props.canceling ? "Canceling…" : "Cancel"}
        </button>
      ) : null}
    </li>
  );
}

function TerminalView(props: {
  runs: RunRef[];
  items: Item[];
  liveToolOutputs: Record<string, LiveToolOutput>;
}) {
  const commands = useMemo(
    () => buildTerminalCommands(props.runs, props.items, props.liveToolOutputs),
    [props.items, props.liveToolOutputs, props.runs],
  );
  const scroll = useRef<HTMLDivElement>(null);
  const followsTail = useRef(true);
  const [pinned, setPinned] = useState(true);
  const materialVersion = commands
    .map(
      (command) =>
        `${command.id}:${command.item.status}:${command.stdout.length}:${command.stderr.length}:${command.liveOutput?.text.length ?? 0}`,
    )
    .join("|");

  useEffect(() => {
    if (!followsTail.current || scroll.current === null) return;
    const frame = window.requestAnimationFrame(() => {
      scroll.current?.scrollTo({ top: scroll.current.scrollHeight });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [materialVersion]);

  const trackReader = () => {
    const element = scroll.current;
    if (element === null) return;
    const follows =
      element.scrollHeight - element.scrollTop - element.clientHeight < 48;
    followsTail.current = follows;
    setPinned(follows);
  };
  const follow = () => {
    followsTail.current = true;
    setPinned(true);
    scroll.current?.scrollTo({ top: scroll.current.scrollHeight });
  };

  if (commands.length === 0) {
    return (
      <ActivityEmpty
        title="No commands yet"
        detail="Shell tool calls will appear here as a read-only execution log."
      />
    );
  }
  return (
    <div className="terminal-view">
      <header className="terminal-toolbar">
        <span>
          {commands.length} {commands.length === 1 ? "command" : "commands"}
        </span>
        <button
          type="button"
          className="quiet-action"
          onClick={follow}
          disabled={pinned}
        >
          {pinned ? "Following output" : "Follow output"}
        </button>
      </header>
      <div className="terminal-log" ref={scroll} onScroll={trackReader}>
        {commands.map((command, index) => (
          <CommandRecord key={command.id} command={command} index={index + 1} />
        ))}
      </div>
    </div>
  );
}

function CommandRecord({
  command,
  index,
}: {
  command: TerminalCommand;
  index: number;
}) {
  const output =
    command.stdout || command.stderr || command.liveOutput?.text || "";
  const running = command.item.status === "running";
  return (
    <details className="terminal-command" open={running}>
      <summary>
        <span className="terminal-prompt" aria-hidden="true">$</span>
        <code>{command.command}</code>
        <span
          className="terminal-command-state"
          data-status={command.item.status}
        >
          {running
            ? "running"
            : command.exitCode !== undefined
              ? `exit ${command.exitCode}`
              : toolStatusLabel(command.item)}
        </span>
      </summary>
      <div className="terminal-material">
        <header>
          <span>Command {index}</span>
          <span title={command.run.id}>{shortIdentity(command.run.id)}</span>
          {command.item.durationMillis !== undefined ? (
            <span>{formatToolDuration(command.item.durationMillis)}</span>
          ) : null}
          {command.killed ? <span>killed</span> : null}
        </header>
        {command.liveOutput?.truncated ? (
          <p className="terminal-truncated">
            Earlier live output was dropped to keep this preview bounded.
          </p>
        ) : null}
        {output ? (
          <>
            {command.stdout ? <pre>{command.stdout}</pre> : null}
            {command.stderr ? (
              <pre className="terminal-stderr">{command.stderr}</pre>
            ) : null}
            {!command.stdout && !command.stderr && command.liveOutput ? (
              <pre>{command.liveOutput.text}</pre>
            ) : null}
          </>
        ) : (
          <p className="terminal-empty-output">
            {running
              ? "Waiting for command output…"
              : "Command produced no captured output."}
          </p>
        )}
      </div>
    </details>
  );
}

function SummaryView(props: {
  runs: RunRef[];
  items: Item[];
  liveToolOutputs: Record<string, LiveToolOutput>;
}) {
  const summary = useMemo(
    () => buildLatestRunSummary(props.runs, props.items, props.liveToolOutputs),
    [props.items, props.liveToolOutputs, props.runs],
  );
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  useEffect(() => setCopyState("idle"), [summary?.root.id]);

  if (summary === undefined) {
    return (
      <ActivityEmpty
        title="No run summary yet"
        detail="The latest root run and its delegated tree will be summarized here."
      />
    );
  }
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(summaryAsText(summary));
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  };
  return (
    <div className="activity-scroll run-summary-view">
      <header className="run-summary-heading">
        <div>
          <span className="eyebrow">Latest run tree</span>
          <h3 title={summary.root.id}>{shortIdentity(summary.root.id)}</h3>
        </div>
        <button
          type="button"
          className="secondary-action"
          onClick={() => void copy()}
        >
          {copyState === "copied" ? "Copied" : "Copy summary"}
        </button>
      </header>
      {copyState === "failed" ? (
        <p className="activity-integrity" role="alert">
          The system clipboard is unavailable. Nothing was copied.
        </p>
      ) : null}
      <SummaryFacts summary={summary} />
      <SummarySection
        title="Changed files"
        empty="No file changes were recorded."
        omitted={summary.omitted.changes}
      >
        {summary.changes.map((change) => (
          <li key={change.path}>
            <span className="summary-action" data-action={change.action}>
              {change.action}
            </span>
            <code>{change.path}</code>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title="Read files"
        empty="No file reads were recorded."
        omitted={summary.omitted.readFiles}
      >
        {summary.readFiles.map((path) => (
          <li key={path}>
            <code>{path}</code>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title="Commands"
        empty="No commands were run."
        omitted={summary.omitted.commands}
      >
        {summary.commands.map((command) => (
          <li key={command.id}>
            <code>{command.command}</code>
            <span>
              {command.exitCode === undefined
                ? toolStatusLabel(command.item)
                : `exit ${command.exitCode}`}
            </span>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title="Approvals"
        empty="No approval boundary was crossed."
        omitted={summary.omitted.approvals}
      >
        {summary.approvals.map((approval, index) => (
          <li key={`${approval.tool}:${approval.subject ?? ""}:${index}`}>
            <span>{approval.decision}</span>
            <strong>{approval.tool}</strong>
            {approval.subject ? <code>{approval.subject}</code> : null}
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title="Errors"
        empty="No errors were recorded."
        omitted={summary.omitted.errors}
        tone="error"
      >
        {summary.errors.map((error, index) => (
          <li key={`${error.source}:${index}`}>
            <strong>{error.source}</strong>
            <span>{error.detail}</span>
          </li>
        ))}
      </SummarySection>
    </div>
  );
}

function SummaryFacts({ summary }: { summary: SessionRunSummary }) {
  const totalTokens =
    summary.usage.inputTokens === undefined &&
    summary.usage.outputTokens === undefined
      ? undefined
      : (summary.usage.inputTokens ?? 0) + (summary.usage.outputTokens ?? 0);
  return (
    <dl className="run-summary-facts">
      <Fact
        label="Status"
        value={humanize(summary.status)}
        status={summary.status}
      />
      <Fact label="Runs" value={String(summary.runs.length)} />
      <Fact label="Steps" value={String(summary.steps)} />
      <Fact
        label="Active time"
        value={formatToolDuration(summary.activeDurationMillis)}
      />
      <Fact
        label="Tokens"
        value={totalTokens === undefined ? "Unknown" : formatNumber(totalTokens)}
      />
      <Fact
        label="Cost"
        value={
          summary.usage.costUsd === undefined
            ? "Unknown"
            : `$${summary.usage.costUsd.toFixed(4)}`
        }
      />
    </dl>
  );
}

function Fact(props: { label: string; value: string; status?: string }) {
  return (
    <div>
      <dt>{props.label}</dt>
      <dd data-status={props.status}>{props.value}</dd>
    </div>
  );
}

function SummarySection(props: {
  title: string;
  empty: string;
  omitted: number;
  tone?: "error";
  children: ReactNode;
}) {
  const hasChildren = Array.isArray(props.children)
    ? props.children.length > 0
    : props.children !== null;
  return (
    <section className="run-summary-section" data-tone={props.tone}>
      <h4>{props.title}</h4>
      {hasChildren ? <ul>{props.children}</ul> : <p>{props.empty}</p>}
      {props.omitted > 0 ? (
        <small>And {props.omitted} more bounded from this view.</small>
      ) : null}
    </section>
  );
}

function ActivityEmpty(props: { title: string; detail: string }) {
  return (
    <section className="activity-empty">
      <span className="activity-empty-mark" aria-hidden="true">◇</span>
      <h3>{props.title}</h3>
      <p>{props.detail}</p>
    </section>
  );
}

function OccurredAt({ value }: { value?: string }) {
  if (!value) return null;
  return <time dateTime={value}>{formatTime(value)}</time>;
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function formatNumber(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
  return value.toLocaleString();
}

function humanize(value: string) {
  return value.replaceAll(/([a-z])([A-Z])/g, "$1 $2").replaceAll("_", " ");
}

function shortIdentity(identity: string) {
  return identity.length <= 18
    ? identity
    : `${identity.slice(0, 10)}…${identity.slice(-6)}`;
}
