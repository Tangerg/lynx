// search_tools preview — which tools the agent just pulled into reach.
//
// The result names them grouped by where they came from (built-in, an MCP server,
// an A2A agent), and that grouping is the useful part: "it loaded three Sentry
// tools" is a different fact from "it loaded three tools".

import type { ToolPreviewProps } from "@/plugins/sdk";
import { Badge } from "@/ui";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectToolSearchGroups } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { toolSearchPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

function ToolSearchPreview({ tool }: ToolPreviewProps) {
  const groups = projectToolSearchGroups(tool.result);
  if (groups.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.noTools"
        />
      </div>
    );
  }
  return (
    <div className="max-h-60 overflow-y-auto pt-1">
      {groups.map((group) => (
        <div key={group.source} className="flex items-start gap-2.5 py-1">
          <span className="w-20 shrink-0 truncate pt-0.5 text-ui-sm text-fg-faint">
            {group.source}
          </span>
          <div className="flex min-w-0 flex-wrap gap-1">
            {group.names.map((name) => (
              <Badge key={name} className="font-mono">
                {name}
              </Badge>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

export const toolSearchPreviewPlugin = definePlugin({
  name: "scopeapp.builtin.tool-search-preview",
  setup(ctx) {
    for (const preview of toolSearchPreview(ToolSearchPreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
