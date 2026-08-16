// Skill previews — the catalog, loaded instructions, one resource and a proposal
// are separate components. Text-shaped tools share typography, not identity.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectSkillPreview } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { skillToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

function SkillCatalogPreview({ tool, onOpenView }: ToolPreviewProps) {
  const entries = projectSkillPreview(tool.result);
  if (entries.length === 0) {
    return (
      <div className={TEXT_PREVIEW_CLASS}>
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.noTools"
        />
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

function SkillTextPreview({ tool, onOpenView }: ToolPreviewProps) {
  const lines = resultLines(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {lines.length > 0 ? (
        <div className="whitespace-pre-wrap break-words text-fg-soft">
          {lines.slice(0, INLINE_PREVIEW_ROW_LIMIT).join("\n")}
        </div>
      ) : (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.loadingTools"
          idle="tools.preview.idle.empty"
        />
      )}
      <PreviewOverflow count={lines.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewText" onClick={onOpenView} />
    </div>
  );
}

function LoadedSkillPreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

function SkillResourcePreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

function SkillProposalPreview(props: ToolPreviewProps) {
  return <SkillTextPreview {...props} />;
}

export const skillPreview = definePlugin({
  name: "lyra.builtin.skill-preview",
  setup(ctx) {
    for (const preview of skillToolPreviews({
      list_skills: SkillCatalogPreview,
      load_skill: LoadedSkillPreview,
      read_skill_resource: SkillResourcePreview,
      propose_skill: SkillProposalPreview,
    })) {
      ctx.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
