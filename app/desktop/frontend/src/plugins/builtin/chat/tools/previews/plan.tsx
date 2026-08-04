// set_plan preview — the plan the agent just wrote, as the checklist it is.
//
// The same StepRow the Plan panel and the progress banner use, so the plan reads
// identically wherever it appears: the tool row shows what this call changed it to,
// the panel shows what it is now, and neither has its own idea of what a step
// looks like.
//
// Read from the call's ARGUMENTS, through the same projection the row's step and
// ratio come from. The runtime also renders the plan as `[x] …` lines for the model,
// and this used to parse those back — a second answer to "what are the steps",
// derived from a rendering rather than from the data, and one that would go quiet
// the day the marks changed.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { StepRow } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { planStepsFromToolArgs } from "@/plugins/builtin/agent/public/plan";
import { planToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function PlanUpdatePreview({ tool }: ToolPreviewProps) {
  const steps = planStepsFromToolArgs(tool.args);
  if (steps.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        {/* A settled call with no steps is a cleared plan, not an empty answer. */}
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.planning"
          idle="tools.preview.idle.planCleared"
        />
      </div>
    );
  }
  return (
    <div className="max-h-60 overflow-y-auto pt-1">
      {steps.map((step) => (
        <StepRow key={step.id} state={step.status}>
          {step.text}
        </StepRow>
      ))}
    </div>
  );
}

export const planPreview = definePlugin({
  name: "lyra.builtin.plan-preview",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of planToolPreview(PlanUpdatePreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
