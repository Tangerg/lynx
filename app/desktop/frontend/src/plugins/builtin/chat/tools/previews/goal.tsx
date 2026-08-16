import type { ToolPreviewProps } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { Badge } from "@/ui";
import { fmtCost } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { projectGoalToolPreview } from "../application/specialisedPreviewProjections";
import { goalToolPreviews } from "../application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function GoalStatePreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const goal = projectGoalToolPreview(tool.result);
  if (!goal) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.empty"
        />
      </div>
    );
  }

  const axes = [
    {
      label: t("goal.budget.runs"),
      used: String(goal.usage.runs),
      max: goal.budget.runs ? String(goal.budget.runs) : t("goal.budget.uncapped"),
    },
    {
      label: t("goal.budget.cost"),
      used: fmtCost(goal.usage.cost),
      max: goal.budget.cost ? fmtCost(goal.budget.cost) : t("goal.budget.uncapped"),
    },
    {
      label: t("goal.budget.steps"),
      used: String(goal.usage.steps),
      max: goal.budget.steps ? String(goal.budget.steps) : t("goal.budget.uncapped"),
    },
  ];

  return (
    <div className="pt-1">
      <div className="flex min-w-0 items-start gap-2">
        <p className="min-w-0 flex-1 text-ui-sm leading-body text-fg-soft">
          {goal.objective || goal.message}
        </p>
        {goal.status && <Badge className="shrink-0 capitalize">{goal.status}</Badge>}
      </div>
      {goal.objective && (
        <div className="mt-2 grid grid-cols-3 gap-2">
          {axes.map((axis) => (
            <div key={axis.label} className="min-w-0 rounded-sm bg-sunken px-2 py-1.5">
              <div className="truncate text-ui-xs text-fg-faint">{axis.label}</div>
              <div className="truncate font-mono text-ui-xs tabular-nums text-fg-muted">
                {axis.used} / {axis.max}
              </div>
            </div>
          ))}
        </div>
      )}
      {goal.objective && goal.message && (
        <p className="mt-2 text-ui-xs leading-body text-fg-faint">{goal.message}</p>
      )}
    </div>
  );
}

function CreatedGoalPreview(props: ToolPreviewProps) {
  return <GoalStatePreview {...props} />;
}

function CurrentGoalPreview(props: ToolPreviewProps) {
  return <GoalStatePreview {...props} />;
}

function GoalOutcomePreview({ tool }: ToolPreviewProps) {
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {tool.result?.trim() ? (
        <p className="whitespace-pre-wrap break-words font-sans text-ui-sm leading-body text-fg-soft">
          {tool.result}
        </p>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.empty"
        />
      )}
    </div>
  );
}

export const goalPreviews = definePlugin({
  name: "lyra.builtin.goal-previews",
  setup(ctx) {
    for (const preview of goalToolPreviews({
      create_goal: CreatedGoalPreview,
      get_goal: CurrentGoalPreview,
      report_goal_outcome: GoalOutcomePreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
