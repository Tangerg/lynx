// set_plan preview — the plan the agent just wrote, as the checklist it is.
//
// The same StepRow the Plan panel and the progress banner use, so the plan reads
// identically wherever it appears: the tool row shows what this call changed it to,
// the panel shows what it is now, and neither has its own idea of what a step
// looks like. The runtime's marks ([x] / [~] / [ ]) already carry the three states.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { StepRow } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectPlanUpdate } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { planToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function PlanUpdatePreview({ tool }: ToolPreviewProps) {
  const steps = projectPlanUpdate(tool.result);
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
      {steps.map((step, i) => (
        <StepRow key={i} state={step.status}>
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
