// lsp preview family — the runtime exposes ONE `lsp` tool (operation in the
// args) plus a separate `lsp_diagnostics`.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { cn } from "@/lib/classNames";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { projectLspOperation } from "@/plugins/builtin/chat/tools/application/specialisedPreviewProjections";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { lspToolPreviews } from "@/plugins/builtin/chat/tools/application/toolPreviewContributions";
import { INLINE_PREVIEW_ROW_LIMIT, PreviewOverflow, TEXT_PREVIEW_CLASS } from "./previewChrome";

// Result is one line per hit: `path:line:col` (locations) or
// `kind Name (in Container) — path:line:col` (symbols), or a "No X found."
// sentence. Symbol lines split on the em-dash so the location reads as metadata
// next to the symbol.
function LspLocationsPreview({ tool, onOpenView }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {rows.length === 0 && (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.querying"
          idle="tools.preview.idle.empty"
        />
      )}
      {rows.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((row, i) => {
        const sep = row.lastIndexOf(" — ");
        if (sep === -1) {
          return (
            <div
              key={i}
              className="truncate rounded-2xs px-1 py-0.5 text-fg-soft hover:bg-hover transition-colors"
            >
              {row}
            </div>
          );
        }
        return (
          <div
            key={i}
            className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 rounded-2xs px-1 py-0.5 hover:bg-hover transition-colors"
          >
            <span className="truncate text-fg-soft">{row.slice(0, sep)}</span>
            <span className="truncate text-ui-sm text-fg-muted">{row.slice(sep + 3)}</span>
          </div>
        );
      })}
      <PreviewOverflow count={rows.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function LspHoverPreview({ tool, onOpenView }: ToolPreviewProps) {
  const text = tool.result?.trim();
  return (
    <div className={cn(TEXT_PREVIEW_CLASS, "whitespace-pre-wrap break-words text-fg-soft")}>
      {text || (
        <PreviewPlaceholder
          status={tool.status}
          pending="tools.preview.pending.querying"
          idle="tools.preview.idle.empty"
        />
      )}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// `severity path:line:col: message [source]` per line — tint the severity word
// so a wall of diagnostics scans by color.
const SEVERITY_TONE: Record<string, string> = {
  error: "text-negative",
  warning: "text-warning",
};

function LspDiagnosticsPreview({ tool, onOpenView }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <div className={TEXT_PREVIEW_CLASS}>
      {rows.slice(0, INLINE_PREVIEW_ROW_LIMIT).map((row, i) => {
        const space = row.indexOf(" ");
        const severity = space === -1 ? "" : row.slice(0, space);
        const tone = SEVERITY_TONE[severity];
        if (!tone) {
          return (
            <div key={i} className="truncate py-0.5 text-fg-soft">
              {row}
            </div>
          );
        }
        return (
          <div key={i} className="truncate py-0.5 text-fg-soft">
            <span className={cn("font-semibold", tone)}>{severity}</span>
            {row.slice(space)}
          </div>
        );
      })}
      <PreviewOverflow count={rows.length - INLINE_PREVIEW_ROW_LIMIT} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// Pick the hover renderer for hover, locations for every other operation;
// default to locations when the operation isn't visible (args are suppressed
// once the call has a label — see projections.argsText).
function LspPreview(props: ToolPreviewProps) {
  return projectLspOperation(props.tool.args) === "hover" ? (
    <LspHoverPreview {...props} />
  ) : (
    <LspLocationsPreview {...props} />
  );
}

export const lspPreviews = definePlugin({
  name: "lyra.builtin.lsp-previews",
  version: "1.0.0",
  setup({ host }) {
    for (const preview of lspToolPreviews(LspPreview, LspDiagnosticsPreview)) {
      host.extensions.contribute(TOOL_PREVIEW, preview.component, { key: preview.key });
    }
  },
});
