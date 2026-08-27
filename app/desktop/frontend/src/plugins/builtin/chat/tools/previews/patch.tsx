// apply_patch preview — one quiet row per file mutation from this ToolCall's
// own persisted PatchResult. The Runtime does not publish line-level diffs here,
// so this surface never substitutes the current worktree or invents diff stats.

import { useT } from "@/lib/i18n";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectPatchChanges, type PatchChange } from "@/plugins/builtin/agent/public/patchResult";
import { applyPatchToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { FilePath } from "@/ui";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

const STATUS_KEY: Record<PatchChange["status"], string> = {
  added: "tools.patch.created",
  deleted: "tools.patch.deleted",
  modified: "tools.patch.edited",
  moved: "tools.patch.moved",
};

function PatchChangeRow({ change }: { change: PatchChange }) {
  const t = useT();
  return (
    <div
      data-patch-change={change.status}
      className="flex min-w-0 items-center gap-1.5 py-0.5 text-ui-md leading-body"
    >
      <span className="shrink-0 font-sans text-fg-faint">{t(STATUS_KEY[change.status])}</span>
      {change.status === "moved" && change.from ? (
        <span className="flex min-w-0 flex-1 items-center gap-1 text-fg-muted">
          <FilePath path={change.from} className="max-w-[42%]" />
          <span aria-hidden="true" className="shrink-0 text-fg-faint">
            →
          </span>
          <FilePath path={change.path} className="flex-1" />
        </span>
      ) : (
        <FilePath path={change.path} className="flex-1 text-fg-muted" />
      )}
    </div>
  );
}

export function ApplyPatchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const changes = projectPatchChanges(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {changes.length === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.running"
          idle="tools.preview.idle.noChanges"
        />
      )}
      {changes.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((change) => (
        <PatchChangeRow
          key={`${change.status}:${change.from ?? ""}:${change.path}`}
          change={change}
        />
      ))}
      <PreviewOverflow count={changes.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

export const applyPatchPreview = definePlugin({
  name: "scopeapp.builtin.apply-patch-preview",
  setup(ctx) {
    for (const preview of applyPatchToolPreview({ apply_patch: ApplyPatchPreview })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
