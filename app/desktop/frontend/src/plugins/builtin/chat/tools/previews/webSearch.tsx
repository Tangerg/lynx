// web_search preview family — rich title/url/snippet result cards.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { SearchResults } from "@/plugins/builtin/chat/tools/public/previews/SearchResults";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectWebSearchPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { webSearchToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

const MAX_WEB_RESULTS = 8;

function WebSearchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const results = projectWebSearchPreview(tool.result);
  if (results.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.searching"
          idle="tools.preview.idle.noResults"
        />
      </div>
    );
  }
  return (
    <div className="pt-1">
      <SearchResults results={results.slice(0, MAX_WEB_RESULTS)} />
      <PreviewOverflow count={results.length - MAX_WEB_RESULTS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const webSearchPreview = definePlugin({
  name: "lyra.builtin.web-search-preview",
  setup(ctx) {
    for (const preview of webSearchToolPreview(WebSearchPreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
