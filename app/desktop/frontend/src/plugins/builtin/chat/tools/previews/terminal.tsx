// terminal preview — the shell family (shell / read_shell_output / stop_shell), all
// terminal-style plain text.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { shellToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

function ShellPreview({ tool, onOpenView }: ToolPreviewProps) {
  return (
    <div>
      {/* `tool.result` is the authoritative merged output — reconciled from the
          completed Item, with the toolOutput delta stream standing in while the
          command runs (projections.ts + API.md §4.4.1). */}
      <ToolOutputPanel
        output={tool.result}
        status={tool.status}
        idleLabel="tools.preview.idle.noOutput"
      />
      <PreviewFoot label="tools.preview.openTerminal" onClick={onOpenView} />
    </div>
  );
}

// Previews are keyed by the tool ROUTING KEY = the wire tool `name` (§4.4 /
// §4.4.2 display conventions).
export const shellPreview = definePlugin({
  name: "lyra.builtin.shell",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of shellToolPreviews(ShellPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
