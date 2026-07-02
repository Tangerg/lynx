// web_search preview family — rich title/url/snippet result cards (the search
// rendering is the tool preview; see preview-blocks/viewBlocks).

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { SearchResults } from "@/plugins/builtin/chat/tools/public/previews/SearchResults";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { webSearchPreviewResults } from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { Overflow, PREVIEW_WRAP } from "./shared";

const MAX_WEB_RESULTS = 8;

function WebSearchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const results = webSearchPreviewResults(tool.result);
  if (results.length === 0) {
    return (
      <div className={PREVIEW_WRAP}>
        <PreviewPlaceholder status={tool.status} pending="Searching…" idle="(no results)" />
      </div>
    );
  }
  return (
    <div className="bg-canvas px-3.5 pt-2.5 pb-2">
      <SearchResults results={results.slice(0, MAX_WEB_RESULTS)} />
      <Overflow count={results.length - MAX_WEB_RESULTS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const webSearchPreview = definePlugin({
  name: "lyra.builtin.web-search-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, WebSearchPreview, { key: "web_search" });
  },
});
