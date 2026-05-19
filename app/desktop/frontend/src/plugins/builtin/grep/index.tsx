// Built-in plugin: grep result preview.

import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useGrep } from "@/lib/queries";
import { definePlugin, type ToolPreviewProps } from "@/plugins/sdk";

function GrepPreview({ onOpenInspector }: ToolPreviewProps) {
  const { data } = useGrep();
  const matches = data?.matches ?? [];
  const total = data?.total ?? matches.length;
  const visible = matches.slice(0, 4);
  const overflow = total - visible.length;

  return (
    <div className="tool-preview">
      <div className="grep-preview">
        {visible.map((m, i) => (
          <div key={i} className="grep-line">
            <span className="path">{m.path}</span>
            <span className="match">{m.match}</span>
          </div>
        ))}
        {overflow > 0 && <div className="grep-line muted">… {overflow} more matches</div>}
      </div>
      <PreviewFoot label="View all matches" onClick={onOpenInspector} />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("grep", GrepPreview);
  },
});
