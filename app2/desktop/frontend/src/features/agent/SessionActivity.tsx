import {
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";

import type {
  Item,
  PendingInterruptSet,
  RunRef,
} from "@lyra/runtime-contract";

import {
  useLocalization,
  type MessageKey,
} from "../localization/Localization";
import type { LiveToolOutput, SessionActivityView } from "./agentSessionTypes";
import {
  approvalDecisionLabel,
  buildLatestRunSummary,
  buildTerminalCommands,
  buildTimeline,
  changeActionLabel,
  runStatus,
  runStatusLabel,
  summaryAsText,
  type SessionRunSummary,
  type TerminalCommand,
  type TimelineEntry,
} from "./sessionActivityModel";
import { formatToolDuration, toolStatusLabel } from "./toolPresentation";
import { useFollowScroll } from "./useFollowScroll";

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

const views: { id: SessionActivityView; label: MessageKey }[] = [
  { id: "overview", label: "activity.overview" },
  { id: "timeline", label: "activity.timeline" },
  { id: "terminal", label: "activity.terminal" },
  { id: "summary", label: "activity.summary" },
];

export function SessionActivity(props: SessionActivityProps) {
  const { t } = useLocalization();
  return (
    <section className="session-activity" aria-label={t("activity.label")}>
      <nav className="session-view-switch" aria-label={t("activity.views")}>
        {views.map((view) => (
          <button
            key={view.id}
            type="button"
            aria-current={props.view === view.id ? "page" : undefined}
            onClick={() => props.onViewChange(view.id)}
          >
            {t(view.label)}
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
  const { t } = useLocalization();
  const groups = useMemo(
    () => buildTimeline(props.runs, props.items, props.interrupts, t),
    [props.interrupts, props.items, props.runs, t],
  );
  if (groups.length === 0) {
    return (
      <ActivityEmpty
        title={t("activity.noTimeline")}
        detail={t("activity.noTimelineDetail")}
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
                {groupIndex === 0
                  ? t("activity.latestRunTree")
                  : t("activity.earlierRunTree")}
              </span>
              <strong title={group.root.id}>
                {shortIdentity(group.root.id)}
              </strong>
            </div>
            <span className="run-state-chip" data-status={runStatus(group.root)}>
              {runStatusLabel(runStatus(group.root), t)}
            </span>
          </header>
          <p className="run-tree-facts">
            {t(
              group.runs.length === 1
                ? "activity.runCountOne"
                : "activity.runCountMany",
              { count: group.runs.length },
            )}
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
  const { t } = useLocalization();
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
          <strong>
            {approval
              ? t("activity.approvalRequested")
              : t("activity.inputRequested")}
          </strong>
          {entry.tool?.subject ? <code>{entry.tool.subject}</code> : null}
          <span className="timeline-row-facts">
            {entry.tool ? <small>{entry.tool.title}</small> : null}
            <small>{t("activity.pending")}</small>
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
            <small>{toolStatusLabel(entry.item, t)}</small>
            {entry.item.approvalDecision ? (
              <small>
                {approvalDecisionLabel(entry.item.approvalDecision, t)}
              </small>
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
        ? t("activity.runStarted")
        : t("activity.delegatedRunStarted")
      : entry.kind === "runWaiting"
        ? t("activity.runWaiting")
        : t("activity.runState", {
            status: runStatusLabel(runStatus(entry.run), t),
          });
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
            <small>
              {t("activity.stepCount", { count: entry.run.metrics.steps })}
            </small>
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
          {props.canceling ? t("activity.canceling") : t("activity.cancel")}
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
  const { t } = useLocalization();
  const commands = useMemo(
    () =>
      buildTerminalCommands(
        props.runs,
        props.items,
        props.liveToolOutputs,
        t,
      ),
    [props.items, props.liveToolOutputs, props.runs, t],
  );
	const materialVersion = useMemo(
		() =>
			commands
				.map(
					(command) =>
						`${command.id}:${command.item.status}:${command.stdout.length}:${command.stderr.length}:${command.liveOutput?.text.length ?? 0}`,
				)
				.join("|"),
		[commands],
	);
	const reader = useFollowScroll(materialVersion, 48, commands.length > 0);

  if (commands.length === 0) {
    return (
      <ActivityEmpty
        title={t("activity.noCommands")}
        detail={t("activity.noCommandsDetail")}
      />
    );
  }
  return (
    <div className="terminal-view">
      <header className="terminal-toolbar">
        <span>
          {t(
            commands.length === 1
              ? "activity.commandCountOne"
              : "activity.commandCountMany",
            { count: commands.length },
          )}
        </span>
        <button
          type="button"
          className="quiet-action"
          onClick={reader.follow}
          disabled={reader.following}
          data-new-material={reader.hasNewMaterial || undefined}
          aria-live="polite"
        >
          {reader.following
            ? t("activity.followingOutput")
            : reader.hasNewMaterial
              ? t("activity.newOutputBelow")
              : t("activity.followOutput")}
        </button>
      </header>
		<div
			className="terminal-log"
			ref={reader.viewportRef}
			onScroll={reader.onScroll}
		>
			<div className="terminal-log-material" ref={reader.contentRef}>
          {commands.map((command, index) => (
            <CommandRecord key={command.id} command={command} index={index + 1} />
          ))}
			</div>
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
  const { t } = useLocalization();
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
            ? t("activity.running")
            : command.exitCode !== undefined
              ? t("tool.exitCode", { code: command.exitCode })
              : toolStatusLabel(command.item, t)}
        </span>
      </summary>
      <div className="terminal-material">
        <header>
          <span>{t("activity.commandNumber", { number: index })}</span>
          <span title={command.run.id}>{shortIdentity(command.run.id)}</span>
          {command.item.durationMillis !== undefined ? (
            <span>{formatToolDuration(command.item.durationMillis)}</span>
          ) : null}
          {command.killed ? <span>{t("activity.killed")}</span> : null}
        </header>
        {command.liveOutput?.truncated ? (
          <p className="terminal-truncated">
            {t("activity.liveOutputTruncated")}
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
              ? t("activity.waitingCommandOutput")
              : t("activity.noCapturedOutput")}
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
  const { t } = useLocalization();
  const summary = useMemo(
    () =>
      buildLatestRunSummary(
        props.runs,
        props.items,
        props.liveToolOutputs,
        t,
      ),
    [props.items, props.liveToolOutputs, props.runs, t],
  );
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  useEffect(() => setCopyState("idle"), [summary?.root.id]);

  if (summary === undefined) {
    return (
      <ActivityEmpty
        title={t("activity.noSummary")}
        detail={t("activity.noSummaryDetail")}
      />
    );
  }
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(summaryAsText(summary, t));
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  };
  return (
    <div className="activity-scroll run-summary-view">
      <header className="run-summary-heading">
        <div>
          <span className="eyebrow">{t("activity.latestRunTree")}</span>
          <h3 title={summary.root.id}>{shortIdentity(summary.root.id)}</h3>
        </div>
        <button
          type="button"
          className="secondary-action"
          onClick={() => void copy()}
        >
          {copyState === "copied"
            ? t("activity.copied")
            : t("activity.copySummary")}
        </button>
      </header>
      {copyState === "failed" ? (
        <p className="activity-integrity" role="alert">
          {t("activity.clipboardUnavailable")}
        </p>
      ) : null}
      <SummaryFacts summary={summary} />
      <SummarySection
        title={t("activity.changedFiles")}
        empty={t("activity.noChangedFiles")}
        omitted={summary.omitted.changes}
      >
        {summary.changes.map((change) => (
          <li key={change.path}>
            <span className="summary-action" data-action={change.action}>
              {changeActionLabel(change.action, t)}
            </span>
            <code>{change.path}</code>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title={t("activity.readFiles")}
        empty={t("activity.noReadFiles")}
        omitted={summary.omitted.readFiles}
      >
        {summary.readFiles.map((path) => (
          <li key={path}>
            <code>{path}</code>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title={t("activity.commands")}
        empty={t("activity.noCommandsRecorded")}
        omitted={summary.omitted.commands}
      >
        {summary.commands.map((command) => (
          <li key={command.id}>
            <code>{command.command}</code>
            <span>
              {command.exitCode === undefined
                ? toolStatusLabel(command.item, t)
                : t("tool.exitCode", { code: command.exitCode })}
            </span>
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title={t("activity.approvals")}
        empty={t("activity.noApprovals")}
        omitted={summary.omitted.approvals}
      >
        {summary.approvals.map((approval, index) => (
          <li key={`${approval.tool}:${approval.subject ?? ""}:${index}`}>
            <span>{approvalDecisionLabel(approval.decision, t)}</span>
            <strong>{approval.tool}</strong>
            {approval.subject ? <code>{approval.subject}</code> : null}
          </li>
        ))}
      </SummarySection>
      <SummarySection
        title={t("activity.errors")}
        empty={t("activity.noErrors")}
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
  const { formatNumber, t } = useLocalization();
  const totalTokens =
    summary.usage.inputTokens === undefined &&
    summary.usage.outputTokens === undefined
      ? undefined
      : (summary.usage.inputTokens ?? 0) + (summary.usage.outputTokens ?? 0);
  return (
    <dl className="run-summary-facts">
      <Fact
        label={t("activity.status")}
        value={runStatusLabel(summary.status, t)}
        status={summary.status}
      />
      <Fact label={t("activity.runs")} value={formatNumber(summary.runs.length)} />
      <Fact label={t("activity.steps")} value={formatNumber(summary.steps)} />
      <Fact
        label={t("activity.activeTime")}
        value={formatToolDuration(summary.activeDurationMillis)}
      />
      <Fact
        label={t("activity.tokens")}
        value={totalTokens === undefined ? t("activity.unknown") : formatNumber(totalTokens)}
      />
      <Fact
        label={t("activity.cost")}
        value={
          summary.usage.costUsd === undefined
            ? t("activity.unknown")
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
  const { t } = useLocalization();
  const hasChildren = Array.isArray(props.children)
    ? props.children.length > 0
    : props.children !== null;
  return (
    <section className="run-summary-section" data-tone={props.tone}>
      <h4>{props.title}</h4>
      {hasChildren ? <ul>{props.children}</ul> : <p>{props.empty}</p>}
      {props.omitted > 0 ? (
        <small>{t("activity.omitted", { count: props.omitted })}</small>
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
  const { formatDateTime } = useLocalization();
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return <time dateTime={value}>{value}</time>;
  return <time dateTime={value}>{formatDateTime(date, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })}</time>;
}

function shortIdentity(identity: string) {
  return identity.length <= 18
    ? identity
    : `${identity.slice(0, 10)}…${identity.slice(-6)}`;
}
