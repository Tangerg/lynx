// Built-in plugin: terminal output preview for `bash` tool calls.
//
// Lives in the plugin directory because we treat built-ins identically to
// third-party — they have to go through the same SDK to prove the surface
// is sufficient.

import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useTerminal } from "@/lib/queries";
import { definePlugin, type ToolPreviewProps } from "@/plugins/sdk";

const MAX_LINES = 9;

function BashPreview({ onOpenInspector }: ToolPreviewProps) {
  const { data: lines } = useTerminal();
  return (
    <div className="tool-preview term">
      {(lines ?? []).slice(0, MAX_LINES).map((l, i) => (
        <span key={i} className={l.kind}>{l.text}</span>
      ))}
      <PreviewFoot label="Open in inspector" onClick={onOpenInspector} />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("bash", BashPreview);
  },
});
