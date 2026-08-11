// diff preview — edit / write. Prefers the call-scoped patch (FileEdit.diff),
// falling back to the whole-worktree diff via useDiffToolPreview.

import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { cn } from "@/lib/classNames";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useDiffToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewQueries";
import { diffToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { TEXT_PREVIEW_CLASS } from "./previewChrome";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";

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
  const t = useT();
  // Prefer THIS edit's call-scoped patch (FileEdit.diff, §12.1 C) — exactly
  // what the edit changed. Fall back to the whole-worktree diff for a `write`
  // (no call-scoped diff) or until the completed item carries one; each file's
  // path becomes a hunk-style separator row so MAX_DIFF_ROWS stays one slice.
  const { rows, truncated, hiddenRows } = useDiffToolPreview(tool, MAX_DIFF_ROWS);
  // A write has no diff rows and no git history to make some from, so the body used to
  // be blank — a row that had just written a file showed a path and a link to a diff
  // that did not exist. What it wrote is in its own arguments.
  const written = rows.length === 0 ? (tool.written ?? []) : [];
  if (rows.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        {written.length === 0 ? (
          <PreviewPlaceholder
            status={tool.status}
            pending="tools.preview.pending.running"
            idle="tools.preview.idle.noChanges"
          />
        ) : (
          <>
            <div className="font-mono text-ui-sm leading-body">
              {written.slice(0, MAX_DIFF_ROWS).map((line, i) => (
                <div
                  key={i}
                  className={cn(
                    "grid grid-cols-[1.25rem_minmax(0,1fr)] items-start px-0.5",
                    ROW_STYLE.added.tone,
                  )}
                >
                  <span className={cn("text-center select-none", ROW_STYLE.added.meta)}>+</span>
                  <span className="min-w-0 whitespace-pre-wrap wrap-anywhere">{line}</span>
                </div>
              ))}
            </div>
            {(tool.writtenLines ?? written.length) > MAX_DIFF_ROWS && (
              <div className="pt-1 text-fg-faint">
                …{" "}
                {t("tools.overflow.lines2", {
                  count: (tool.writtenLines ?? written.length) - MAX_DIFF_ROWS,
                })}
              </div>
            )}
          </>
        )}
        <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      <div className="font-mono text-ui-sm leading-body">
        {rows.slice(0, MAX_DIFF_ROWS).map((row, i) => {
          if (row.type === "hunk") {
            return (
              <div key={i} className="mx-0 mt-1.5 mb-1 px-1.5 py-1 text-ui-sm text-fg-faint">
                {row.text}
              </div>
            );
          }
          const style = ROW_STYLE[row.type];
          return (
            <div
              key={i}
              className={cn("grid grid-cols-[18px_minmax(0,1fr)] items-start px-0.5", style.tone)}
            >
              <span className={cn("text-center text-ui-sm select-none", style.meta)}>
                {style.sign}
              </span>
              <span className={cn("min-w-0 whitespace-pre-wrap wrap-anywhere", style.codeTone)}>
                {row.code}
              </span>
            </div>
          );
        })}
        {(hiddenRows > 0 || truncated) && (
          <div className="text-fg-faint">
            {hiddenRows > 0 && `… ${t("tools.overflow.rows", { count: hiddenRows })}`}
            {truncated && ` · ${t("tools.overflow.truncated")}`}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

function EditPreview(props: ToolPreviewProps) {
  return <DiffPreview {...props} />;
}

function WritePreview(props: ToolPreviewProps) {
  return <DiffPreview {...props} />;
}

function ApplyPatchPreview(props: ToolPreviewProps) {
  return <DiffPreview {...props} />;
}

export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of diffToolPreviews({
      edit: EditPreview,
      write: WritePreview,
      apply_patch: ApplyPatchPreview,
    })) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
