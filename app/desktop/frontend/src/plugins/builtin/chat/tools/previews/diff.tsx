// diff preview — edit / write. Prefers the call-scoped patch (FileEdit.diff),
// falling back to the whole-worktree diff via useDiffToolPreview.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useDiffToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewData";
import { diffToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { PREVIEW_WRAP } from "./shared";

const MAX_DIFF_ROWS = 8;

// Per-line presentation keyed by row type — one lookup beats four parallel
// ternary chains switching on the same field. (The full DiffView in the
// workspace plugin keeps its own narrower table: it highlights code via shiki
// instead of carrying a flat `codeTone`, so the two don't share a module.)
const ROW_STYLE: Record<
  "added" | "deleted" | "context",
  { tone: string; meta: string; codeTone: string; sign: string }
> = {
  added: {
    tone: "bg-[var(--color-diff-added-tint)]",
    meta: "text-[var(--color-diff-added-meta)]",
    codeTone: "text-[var(--color-diff-added-code)]",
    sign: "+",
  },
  deleted: {
    tone: "bg-[var(--color-diff-deleted-tint)]",
    meta: "text-[var(--color-diff-deleted-meta)]",
    codeTone: "text-[var(--color-diff-deleted-code)]",
    sign: "−",
  },
  context: { tone: "", meta: "text-fg-faint", codeTone: "text-fg-soft", sign: " " },
};

function DiffPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Prefer THIS edit's call-scoped patch (FileEdit.diff, §12.1 C) — exactly
  // what the edit changed. Fall back to the whole-worktree diff for a `write`
  // (no call-scoped diff) or until the completed item carries one; each file's
  // path becomes a hunk-style separator row so MAX_DIFF_ROWS stays one slice.
  const { rows, truncated, hiddenRows } = useDiffToolPreview(tool, MAX_DIFF_ROWS);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {rows.slice(0, MAX_DIFF_ROWS).map((row, i) => {
          if (row.type === "hunk") {
            return (
              <div key={i} className="mx-0 mt-1.5 mb-1 px-1.5 py-1 text-[11px] text-fg-faint">
                {row.text}
              </div>
            );
          }
          const style = ROW_STYLE[row.type];
          return (
            <div key={i} className={cn("grid grid-cols-[18px_1fr] px-0.5", style.tone)}>
              <span className={cn("text-center text-[11px] select-none", style.meta)}>
                {style.sign}
              </span>
              <span className={cn("whitespace-pre", style.codeTone)}>{row.code}</span>
            </div>
          );
        })}
        {(hiddenRows > 0 || truncated) && (
          <div className="text-fg-faint">
            {hiddenRows > 0 && `… ${hiddenRows} more rows`}
            {truncated && " · truncated by runtime"}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of diffToolPreviews(DiffPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
