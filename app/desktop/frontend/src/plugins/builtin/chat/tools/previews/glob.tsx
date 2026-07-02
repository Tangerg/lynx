// glob preview family — a matched-path list. A glob pattern is not a
// workspace.grep query and GlobResponse carries the paths inline, so it gets its
// own preview rather than riding the grep one.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { globPreviewData } from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { MAX_ROWS, Overflow, PREVIEW_WRAP } from "./shared";

function GlobPreview({ tool, onOpenView }: ToolPreviewProps) {
  const { paths, truncated } = globPreviewData(tool.result);
  return (
    <div className={PREVIEW_WRAP}>
      {paths.length === 0 && (
        <PreviewPlaceholder status={tool.status} pending="Matching…" idle="(no matches)" />
      )}
      {paths.slice(0, MAX_ROWS).map((p) => (
        <div key={p} className="truncate py-0.5 text-fg-soft">
          {p}
        </div>
      ))}
      <Overflow count={paths.length - MAX_ROWS} />
      {truncated && <div className="text-fg-faint">… truncated by runtime</div>}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const globPreview = definePlugin({
  name: "lyra.builtin.glob-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, GlobPreview, { key: "glob" });
  },
});
