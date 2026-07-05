// task (sub-agent) preview family — the result is the sub-agent's final reply.
// The child run itself streams on the same tree (spawnedByItemId); this preview
// is the parent-side summary of what came back.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { taskToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { MAX_ROWS, Overflow, PREVIEW_WRAP } from "./shared";

function TaskPreview({ tool, onOpenView }: ToolPreviewProps) {
  const lines = resultLines(tool.result);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="whitespace-pre-wrap break-words text-fg-soft">
        {lines.slice(0, MAX_ROWS).join("\n") ||
          (tool.status === "running" ? "Sub-agent working…" : "(no reply)")}
      </div>
      <Overflow count={lines.length - MAX_ROWS} />
      <PreviewFoot label="tools.preview.viewReply" onClick={onOpenView} />
    </div>
  );
}

export const taskPreview = definePlugin({
  name: "lyra.builtin.task-preview",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of taskToolPreviews(TaskPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
