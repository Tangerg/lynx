// skill preview family — a list of skill name + description entries, falling
// back to raw text lines when the result isn't the structured catalog.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectSkillPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { skillToolPreview } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function SkillPreview({ tool, onOpenView }: ToolPreviewProps) {
  const entries = projectSkillPreview(tool.result);
  if (entries.length === 0) {
    const lines = resultLines(tool.result);
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <div className="whitespace-pre-wrap break-words text-fg-soft">
          {lines.slice(0, INLINE_PREVIEW_ROW_LIMIT).join("\n") ||
            (tool.status === "running" ? "Loading…" : "(empty)")}
        </div>
        <PreviewOverflow count={lines.length - INLINE_PREVIEW_ROW_LIMIT} />
        <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
      </div>
    );
  }
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {entries.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((s) => (
        <div
          key={s.name}
          className="flex items-baseline gap-2 rounded-2xs px-1 py-0.5 hover:bg-hover transition-colors"
        >
          <code className="shrink-0 rounded-xs bg-surface-2 px-1.5 py-0.5 text-ui-sm text-fg-soft">
            {s.name}
          </code>
          <span className="truncate text-ui-sm text-fg-muted">{s.description}</span>
        </div>
      ))}
      <PreviewOverflow count={entries.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const skillPreview = definePlugin({
  name: "lyra.builtin.skill-preview",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of skillToolPreview(SkillPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
