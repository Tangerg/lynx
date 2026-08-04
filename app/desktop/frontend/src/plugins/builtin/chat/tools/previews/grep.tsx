// grep preview — matches from the call's own result (output_mode honors
// glob/type/context filters a re-query can't reproduce), falling back to the
// workspace.files.search re-query. The query comes off `tool.fn` (search → query, the
// §4.4.2 projection).

import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useGrepToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewQueries";
import { grepToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";

const MAX_GREP_MATCHES = 4;

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const { shown, overflow } = useGrepToolPreview(tool, MAX_GREP_MATCHES);
  // §7.5 no-silent-caps: surface our own preview cap. The runtime's search
  // presentation drops grep's `truncated`, so a server-side cap is no longer a
  // state a tool result can report.
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      <div className="font-mono text-ui-sm leading-body">
        {shown.map((r, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 overflow-hidden rounded-2xs px-1 py-0.5 whitespace-nowrap hover:bg-hover transition-colors"
          >
            <span className="truncate text-ui-sm text-fg-muted">
              <LinkedText text={r.loc} />
            </span>
            <span className="truncate text-fg-soft">{r.text}</span>
          </div>
        ))}
        {overflow > 0 && (
          <div className="pt-1 text-fg-faint">
            … {t("tools.overflow.matches", { count: overflow })}
          </div>
        )}
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
