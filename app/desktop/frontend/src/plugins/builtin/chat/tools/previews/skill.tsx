// skill preview family — a list of skill name + description entries, falling
// back to raw text lines when the result isn't the structured catalog.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { skillPreviewEntries } from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { MAX_ROWS, Overflow, PREVIEW_WRAP } from "./shared";

function SkillPreview({ tool, onOpenView }: ToolPreviewProps) {
  const entries = skillPreviewEntries(tool.result);
  if (entries.length === 0) {
    const lines = resultLines(tool.result);
    return (
      <div className={PREVIEW_WRAP}>
        <div className="whitespace-pre-wrap break-words text-fg-soft">
          {lines.slice(0, MAX_ROWS).join("\n") ||
            (tool.status === "running" ? "Loading…" : "(empty)")}
        </div>
        <Overflow count={lines.length - MAX_ROWS} />
        <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
      </div>
    );
  }
  return (
    <div className={PREVIEW_WRAP}>
      {entries.slice(0, MAX_ROWS).map((s) => (
        <div key={s.name} className="flex items-baseline gap-2 py-0.5">
          <code className="shrink-0 rounded-xs bg-surface-2 px-1 text-[11px] text-fg">
            {s.name}
          </code>
          <span className="truncate text-[11.5px] text-fg-faint">{s.description}</span>
        </div>
      ))}
      <Overflow count={entries.length - MAX_ROWS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const skillPreview = definePlugin({
  name: "lyra.builtin.skill-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, SkillPreview, { key: "skill" });
  },
});
