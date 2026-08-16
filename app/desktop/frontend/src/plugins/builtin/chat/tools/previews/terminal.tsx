// Shell-family previews. They share the terminal material, but each tool owns a
// component and can evolve without changing its siblings' rendering contract.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { ToolOutputPanel } from "@/plugins/builtin/chat/tools/public/previews/ToolOutputPanel";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { shellToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";

function TerminalResult({ tool, onOpenView }: ToolPreviewProps) {
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

function ShellCommandPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

function ShellOutputPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

function StopShellPreview(props: ToolPreviewProps) {
  return <TerminalResult {...props} />;
}

// Previews are keyed by the tool ROUTING KEY = the wire tool `name` (§4.4 /
// §4.4.2 display conventions).
export const shellPreview = definePlugin({
  name: "lyra.builtin.shell",
  setup(ctx) {
    for (const preview of shellToolPreviews({
      shell: ShellCommandPreview,
      read_shell_output: ShellOutputPreview,
      stop_shell: StopShellPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
