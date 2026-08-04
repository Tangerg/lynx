// create_goal / get_goal / report_goal_outcome preview — the autonomous loop's
// standing order and how much of its allowance it has spent.
//
// All three answer the same { goal, message } envelope, so one renderer reads them
// all; what differs is which call wrote it. The budget is the point: a Goal is the
// one thing here the user hands over control to, and "how far can it still go" is
// the question that makes handing it over safe.

import type { ToolPreviewProps } from "@/plugins/sdk";
import type { Tone } from "@/lib/tone";
import { Badge, ProgressBar } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { fmtCost } from "@/lib/format";
import { useT } from "@/lib/i18n";
import {
  projectGoalPreview,
  type GoalPreview,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { goalToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

const STATUS_TONE: Record<string, Tone> = {
  active: "accent",
  paused: "warning",
  blocked: "negative",
};

function GoalToolPreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const goal = projectGoalPreview(tool.result);
  if (!goal) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.goal"
          idle="tools.preview.idle.noGoal"
        />
      </div>
    );
  }
  return (
    <div className="max-h-60 overflow-y-auto pt-1">
      <div className="flex items-start gap-2">
        <span className="min-w-0 flex-1 text-ui-md leading-body text-fg">{goal.objective}</span>
        {goal.status && (
          <Badge tone={STATUS_TONE[goal.status]}>{t(`goal.status.${goal.status}`)}</Badge>
        )}
      </div>
      {/* Why it stopped, when it has. Terminal reasons are the whole point of
          report_goal_outcome, and they are the one field a paused Goal explains
          itself with. */}
      {goal.reason && (
        <div className="mt-1 text-ui-sm leading-body text-fg-muted">{goal.reason}</div>
      )}
      <div className="mt-2.5 flex flex-col gap-1.5">
        <BudgetAxis
          label={t("goal.budget.turns")}
          used={goal.turns.used}
          max={goal.turns.max}
          format={(value) => String(value)}
        />
        <BudgetAxis
          label={t("goal.budget.cost")}
          used={goal.cost.used}
          max={goal.cost.max}
          format={fmtCost}
        />
        <BudgetAxis
          label={t("goal.budget.steps")}
          used={goal.steps.used}
          max={goal.steps.max}
          format={(value) => String(value)}
        />
      </div>
      {goal.message && (
        <div className="mt-2 text-ui-sm leading-body text-fg-muted">{goal.message}</div>
      )}
    </div>
  );
}

/**
 * One axis of the allowance.
 *
 * A zero max is uncapped on that axis (the wire omits it when zero), and an
 * uncapped axis gets no bar — a full-width track under "no limit" reads as
 * "nearly spent", which is the opposite of what it means.
 */
function BudgetAxis({
  label,
  used,
  max,
  format,
}: {
  label: string;
  used: GoalPreview["turns"]["used"];
  max: number;
  format: (value: number) => string;
}) {
  const t = useT();
  return (
    <div className="grid grid-cols-[4.5rem_minmax(0,1fr)_auto] items-center gap-2.5">
      <span className="text-ui-sm text-fg-faint">{label}</span>
      {max > 0 ? (
        <ProgressBar value={(used / max) * 100} className="h-1" />
      ) : (
        <span className="text-ui-sm text-fg-faint">{t("goal.budget.uncapped")}</span>
      )}
      <span className="font-mono text-ui-xs tabular-nums text-fg-muted">
        {max > 0 ? `${format(used)} / ${format(max)}` : format(used)}
      </span>
    </div>
  );
}

export const goalPreview = definePlugin({
  name: "lyra.builtin.goal-preview",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of goalToolPreviews(GoalToolPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
