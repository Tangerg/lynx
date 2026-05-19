// Built-in plugin: unified-diff preview for `edit_file` and `write_file`.

import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useDiff } from "@/lib/queries";
import { definePlugin, type ToolPreviewProps } from "@/plugins/sdk";

const MAX_ROWS = 8;

function DiffPreview({ onOpenInspector }: ToolPreviewProps) {
  const { data: rows } = useDiff();
  return (
    <div className="tool-preview">
      <div className="diff-view-mini">
        {(rows ?? []).slice(0, MAX_ROWS).map((row, i) => {
          if (row.type === "hunk") {
            return <div key={i} className="diff-hunk-head">{row.text}</div>;
          }
          const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
          const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
          return (
            <div key={i} className={`diff-line ${cls}`}>
              <span className="sign">{sign}</span>
              <span className="code">{row.code}</span>
            </div>
          );
        })}
      </div>
      <PreviewFoot label="View full diff in inspector" onClick={onOpenInspector} />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("edit_file", DiffPreview);
    host.tool.registerPreview("write_file", DiffPreview);
  },
});
