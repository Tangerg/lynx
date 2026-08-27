// file preview — read. Reads its query off `tool.fn` (the §4.4.2 projection
// bakes read → path into the display name; see agent fold toolLabel), then
// fetches the lines via useFileToolPreview against the active session's cwd.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useFileToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewQueries";
import { fileToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

const MAX_FILE_LINES = 40;

function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  // cwd = the active session's workspace — the tool ran there, so the
  // preview must read the same tree (the serve dir may be elsewhere).
  const { data: lines } = useFileToolPreview(tool, MAX_FILE_LINES);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      <div className="font-mono text-ui-sm leading-body">
        {(lines ?? []).map((l) => (
          <div
            key={l.lineNumber}
            className="grid grid-cols-[28px_minmax(0,1fr)] items-start gap-2.5 rounded-2xs px-1 transition-colors hover:bg-hover"
          >
            <span className="text-right text-ui-sm text-fg-faint tabular-nums select-none">
              {l.lineNumber}
            </span>
            {/* Wraps rather than clips, for the reason spelled out in DiffView. */}
            <span className="min-w-0 whitespace-pre-wrap wrap-anywhere text-fg-soft">
              {l.text || " "}
            </span>
          </div>
        ))}
      </div>
      <PreviewFoot label="tools.preview.viewFile" onClick={onOpenView} />
    </div>
  );
}

export const file = definePlugin({
  name: "scopeapp.builtin.file",
  setup(ctx) {
    for (const preview of fileToolPreview(FilePreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
