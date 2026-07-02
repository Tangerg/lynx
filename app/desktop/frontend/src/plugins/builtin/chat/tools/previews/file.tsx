// file preview — read. Reads its query off `tool.fn` (the §4.4.2 projection
// bakes read → path into the display name; see agent fold toolLabel), then
// fetches the lines via useFileToolPreview against the active session's cwd.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useFileToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewData";
import { PREVIEW_WRAP } from "./shared";

const MAX_FILE_LINES = 40;

function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  // cwd = the active session's workspace — the tool ran there, so the
  // preview must read the same tree (the serve dir may be elsewhere).
  const { data: lines } = useFileToolPreview(tool, MAX_FILE_LINES);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {(lines ?? []).map((l) => (
          <div key={l.lineNumber} className="grid grid-cols-[28px_1fr] gap-2.5">
            <span className="text-right text-[11px] text-fg-faint select-none">{l.lineNumber}</span>
            <span className="whitespace-pre text-fg-soft">{l.text || " "}</span>
          </div>
        ))}
      </div>
      <PreviewFoot label="tools.preview.viewFile" onClick={onOpenView} />
    </div>
  );
}

export const file = definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, FilePreview, { key: "read" });
  },
});
