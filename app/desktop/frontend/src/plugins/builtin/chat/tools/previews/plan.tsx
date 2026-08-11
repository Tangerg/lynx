import type { ToolPreviewProps } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { planStepsFromToolArgs } from "@/plugins/builtin/agent/public/plan";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { StepRow } from "@/ui";
import { planToolPreviews } from "../application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function PlanModeResult({ tool }: ToolPreviewProps) {
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

function EnterPlanModePreview(props: ToolPreviewProps) {
  return <PlanModeResult {...props} />;
}

function SetPlanPreview({ tool }: ToolPreviewProps) {
  const steps = planStepsFromToolArgs(tool.args);
  return (
    <div className="pt-1">
      {steps.length > 0 ? (
        <ol className="flex flex-col">
          {steps.map((step) => (
            <li key={step.id}>
              <StepRow state={step.status}>{step.text}</StepRow>
            </li>
          ))}
        </ol>
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

function ExitPlanModePreview(props: ToolPreviewProps) {
  return <PlanModeResult {...props} />;
}

export const planPreviews = definePlugin({
  name: "lyra.builtin.plan-previews",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of planToolPreviews({
      enter_plan_mode: EnterPlanModePreview,
      set_plan: SetPlanPreview,
      exit_plan_mode: ExitPlanModePreview,
    })) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
