// glob preview family — a matched-path list. A glob pattern is not a
// workspace.grep query and GlobResponse carries the paths inline, so it gets its
// own preview rather than riding the grep one.

import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectGlobPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { globToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function GlobPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const { paths, truncated } = projectGlobPreview(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {paths.length === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.matching"
          idle="tools.preview.idle.noMatches"
        />
      )}
      {paths.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((p) => (
        <div
          key={p}
          className="truncate rounded-2xs px-1 py-0.5 text-fg-muted hover:bg-hover transition-colors"
        >
          {p}
        </div>
      ))}
      <PreviewOverflow count={paths.length - INLINE_PREVIEW_ROW_LIMIT} />
      {truncated && <div className="text-fg-faint">… {t("tools.overflow.truncated")}</div>}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const globPreview = definePlugin({
  name: "lyra.builtin.glob-preview",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of globToolPreview(GlobPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
