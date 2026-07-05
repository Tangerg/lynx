// grep preview — matches from the call's own result (output_mode honors
// glob/type/context filters a re-query can't reproduce), falling back to the
// workspace.grep re-query. The query comes off `tool.fn` (search → query, the
// §4.4.2 projection).

import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useGrepToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewData";
import { grepToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { PREVIEW_WRAP } from "./shared";

const MAX_GREP_MATCHES = 4;

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  const { shown, overflow, truncated } = useGrepToolPreview(tool, MAX_GREP_MATCHES);
  // §7.5 no-silent-caps: surface both our preview cap and server truncation.
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {shown.map((r, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 overflow-hidden rounded-[4px] px-1 py-0.5 whitespace-nowrap hover:bg-fg/[0.04]"
          >
            <span className="truncate text-[11px] text-fg-muted">
              <LinkedText text={r.loc} />
            </span>
            <span className="truncate text-fg-soft">{r.text}</span>
          </div>
        ))}
        {overflow > 0 && <div className="pt-1 text-fg-faint">… {overflow} more matches</div>}
        {truncated && <div className="pt-1 text-fg-faint">… truncated by runtime</div>}
      </div>
      <PreviewFoot label="tools.preview.viewMatches" onClick={onOpenView} />
    </div>
  );
}

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of grepToolPreview(GrepPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
