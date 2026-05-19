// Built-in plugin: read_file head preview.

import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useFileHead } from "@/lib/queries";
import { definePlugin, type ToolPreviewProps } from "@/plugins/sdk";

function FilePreview({ onOpenInspector }: ToolPreviewProps) {
  const { data: lines } = useFileHead();
  return (
    <div className="tool-preview">
      <div className="file-preview">
        {(lines ?? []).map((l, i) => (
          <div key={i} className={l.muted ? "fp-line muted" : "fp-line"}>
            <span className="ln">{l.ln}</span>
            <span className="code" dangerouslySetInnerHTML={{ __html: l.code }} />
          </div>
        ))}
      </div>
      <PreviewFoot label="View full file" onClick={onOpenInspector} />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("read_file", FilePreview);
  },
});
