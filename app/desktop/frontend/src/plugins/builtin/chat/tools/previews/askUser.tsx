// ask_user preview family — echoes the user's answer once given, otherwise a
// quiet waiting hint (the interactive card lives elsewhere; this is the
// settled-tool summary).

import type { ToolPreviewProps } from "@/plugins/sdk";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { askUserPreviewAnswer } from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { PREVIEW_WRAP } from "./shared";

function AskUserPreview({ tool }: ToolPreviewProps) {
  const answer = askUserPreviewAnswer(tool.result);
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre-wrap break-words")}>
      {answer ? (
        <>
          <span className="text-fg-faint">answer · </span>
          <span className="text-fg-soft">{answer}</span>
        </>
      ) : (
        <span className="text-fg-faint">Waiting for your answer…</span>
      )}
    </div>
  );
}

export const askUserPreview = definePlugin({
  name: "lyra.builtin.ask-user-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, AskUserPreview, { key: "ask_user" });
  },
});
