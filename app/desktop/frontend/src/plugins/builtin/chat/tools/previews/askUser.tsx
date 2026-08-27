// ask_user preview family — echoes the user's answer once given, otherwise a
// quiet waiting hint (the interactive card lives elsewhere; this is the
// settled-tool summary).

import type { ToolPreviewProps } from "@/plugins/sdk";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectAskUserAnswer } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { askUserToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function AskUserPreview({ tool }: ToolPreviewProps) {
  const t = useT();
  const answer = projectAskUserAnswer(tool.result);
  return (
    <div className={cn(TEXT_PREVIEW_CLASS, "whitespace-pre-wrap break-words")}>
      {answer ? (
        <>
          <span className="text-fg-faint">{t("tool.askUser.answerPrefix")}</span>
          <span className="text-fg-soft">{answer}</span>
        </>
      ) : (
        <span className="text-fg-faint">{t("tool.askUser.waiting")}</span>
      )}
    </div>
  );
}

export const askUserPreview = definePlugin({
  name: "scopeapp.builtin.ask-user-preview",
  setup(ctx) {
    for (const preview of askUserToolPreview(AskUserPreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
