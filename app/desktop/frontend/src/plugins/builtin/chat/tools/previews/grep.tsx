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

/**
 * Matches grouped by the file they are in, with a count per file.
 *
 * A flat list answers "what matched"; grouping answers "where", which is the question
 * a search is asked. It also stops the path repeating on every row — the same long
 * prefix five times over, each truncated in the middle, was most of what the preview
 * showed.
 */
function groupByFile(rows: readonly { loc: string; text: string }[]) {
  const groups: { file: string; matches: { line: string; text: string }[] }[] = [];
  for (const row of rows) {
    const cut = row.loc.lastIndexOf(":");
    const file = cut > 0 ? row.loc.slice(0, cut) : row.loc;
    const line = cut > 0 ? row.loc.slice(cut + 1) : "";
    const last = groups[groups.length - 1];
    if (last?.file === file) last.matches.push({ line, text: row.text });
    else groups.push({ file, matches: [{ line, text: row.text }] });
  }
  return groups;
}

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  const t = useT();
  const { shown, overflow } = useGrepToolPreview(tool, MAX_GREP_MATCHES);
  // §7.5 no-silent-caps: surface our own preview cap. The runtime's search
  // presentation drops grep's `truncated`, so a server-side cap is no longer a
  // state a tool result can report.
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      <div className="flex flex-col gap-1.5">
        {groupByFile(shown).map((group) => (
          <div key={group.file}>
            <div className="flex items-baseline gap-2">
              <span className="min-w-0 flex-1 truncate font-mono text-ui-sm text-fg-soft">
                <LinkedText text={group.file} />
              </span>
              {group.matches.length > 1 && (
                <span className="shrink-0 font-mono text-ui-2xs tabular-nums text-fg-faint">
                  {t("tools.grep.matchCount", { count: group.matches.length })}
                </span>
              )}
            </div>
            {group.matches.map((match, index) => (
              <div
                key={index}
                className="grid grid-cols-[3rem_minmax(0,1fr)] items-start gap-2 rounded-2xs px-1 transition-colors hover:bg-hover"
              >
                <span className="text-right font-mono text-ui-2xs tabular-nums text-fg-faint select-none">
                  {match.line}
                </span>
                {/* Preserve the whole matching line; the match may be beyond the prefix. */}
                <span className="min-w-0 whitespace-pre-wrap wrap-anywhere font-mono text-ui-sm text-fg-muted">
                  {match.text}
                </span>
              </div>
            ))}
          </div>
        ))}
        {overflow > 0 && (
          <div className="text-fg-faint">… {t("tools.overflow.matches", { count: overflow })}</div>
        )}
      </div>
      <PreviewFoot label="tools.preview.viewMatches" onClick={onOpenView} />
    </div>
  );
}

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  setup(ctx) {
    for (const preview of grepToolPreview(GrepPreview)) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
