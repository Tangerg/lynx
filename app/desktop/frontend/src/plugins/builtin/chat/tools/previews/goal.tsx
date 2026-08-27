import type { ToolPreviewProps } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { Badge } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { projectGoalToolPreview } from "../application/specialisedPreviewProjections";
import { goalToolPreviews } from "../application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function GoalStatePreview({ tool }: ToolPreviewProps) {
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

  return (
    <div className="pt-1">
      <div className="flex min-w-0 items-start gap-2">
        <p className="min-w-0 flex-1 text-ui-sm leading-body text-fg-soft">
          {goal.objective || goal.message}
        </p>
        {goal.status && <Badge className="shrink-0 capitalize">{goal.status}</Badge>}
      </div>
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
  name: "scopeapp.builtin.goal-previews",
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
