// delegate_task (sub-agent) preview — the result is the sub-agent's final reply.
// The child run itself streams on the same tree (spawnedByItemId); this preview
// is the parent-side summary of what came back.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { delegationToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function TaskPreview({ tool, onOpenView }: ToolPreviewProps) {
  const lines = resultLines(tool.result);
  const reply = lines.slice(0, INLINE_PREVIEW_ROW_LIMIT).join("\n");
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {reply ? (
        <div className="whitespace-pre-wrap break-words text-fg-soft">{reply}</div>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.delegating"
          idle="tools.preview.idle.noReply"
        />
      )}
      <PreviewOverflow count={lines.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewReply" onClick={onOpenView} />
    </div>
  );
}

export const taskPreview = definePlugin({
  name: "lyra.builtin.task-preview",
  setup(ctx) {
    for (const preview of delegationToolPreview(TaskPreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
