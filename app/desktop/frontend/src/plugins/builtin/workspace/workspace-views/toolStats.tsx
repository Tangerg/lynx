// Built-in plugin: "Tool stats" workspace view — where this session's tool time
// went, and what did not come back.
//
// The transcript answers "what happened, in order"; it cannot answer "which tool
// is slow" or "which one keeps failing" without a person counting rows by eye.
// Everything here is derived from tool calls the fold already holds, and the
// durations are the runtime's own measurements — nothing is timed in the client.

import type { IconName } from "@/ui";
import type { ToolStat, ToolStatsSummary } from "../application/toolStats";
import { toolStats, toolTimeShare } from "../application/toolStats";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { Badge, EmptyState, Icon, ProgressBar } from "@/ui";
import { fmtDuration } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

function ToolStatsTab() {
  const t = useT();
  const summary = toolStats(useActiveSessionToolCalls());

  return (
    <WorkspaceViewLayout
      icon="chart"
      titleStrong
      title="toolStats.title"
      sub={
        summary.calls > 0
          ? t("toolStats.summary", {
              calls: summary.calls,
              duration: fmtDuration(summary.totalMs),
            })
          : undefined
      }
      scrollClassName="py-1"
    >
      {summary.rows.length === 0 ? (
        <EmptyState
          icon="chart"
          title={t("toolStats.empty.title")}
          sub={t("toolStats.empty.sub")}
        />
      ) : (
        summary.rows.map((row) => <ToolStatRow key={row.name} row={row} summary={summary} />)
      )}
    </WorkspaceViewLayout>
  );
}

function ToolStatRow({ row, summary }: { row: ToolStat; summary: ToolStatsSummary }) {
  const t = useT();
  // Through the extension point, not through chat/tools' resolver: this view is a
  // foreign context, and importing that resolver made the two contexts mutually
  // dependent (check-builtin-contexts caught the cycle). The built-in icons are
  // contributed into this same registry at startup, so the answer is identical.
  const icon = (lookupExtensionByKey(TOOL_ICON, row.name) as IconName | undefined) ?? "lightning";

  return (
    <div className="px-3.5 py-2">
      <div className="flex min-w-0 items-baseline gap-2">
        <Icon name={icon} size="sm" className="shrink-0 self-center text-fg-muted" />
        <span className="min-w-0 flex-1 truncate text-ui-md text-fg">{row.name}</span>
        {/* The two ways a call does not deliver, kept apart: a denial is a
            person saying no, and showing it as a failure would make an approval
            policy read as a broken tool. */}
        {row.failed > 0 && (
          <Badge tone="negative">{t("toolStats.failed", { n: row.failed })}</Badge>
        )}
        {row.denied > 0 && <Badge tone="warning">{t("toolStats.denied", { n: row.denied })}</Badge>}
        <span className="shrink-0 font-mono text-ui-xs tabular-nums text-fg-muted">
          {row.timed > 0 ? fmtDuration(row.totalMs) : "—"}
        </span>
      </div>
      <div className="mt-1 flex items-center gap-2.5">
        <ProgressBar value={toolTimeShare(row, summary) * 100} className="h-1 flex-1" />
        <span className="shrink-0 text-ui-sm text-fg-faint">
          {t("toolStats.calls", { n: row.calls })}
          {row.timed > 0 &&
            ` · ${t("toolStats.slowest", { duration: fmtDuration(row.slowestMs) })}`}
        </span>
      </div>
    </div>
  );
}

export const toolStatsView = defineWorkspaceView({
  id: "tool-stats",
  title: "toolStats.title",
  icon: "chart",
  order: 150,
  splittable: true,
  component: ToolStatsTab,
});
