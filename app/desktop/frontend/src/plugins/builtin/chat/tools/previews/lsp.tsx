// lsp preview family — the runtime exposes ONE `lsp` tool (operation in the
// args) plus a separate `lsp_diagnostics`.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { lspPreviewOperation } from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { MAX_ROWS, Overflow, PREVIEW_WRAP } from "./shared";

// Result is one line per hit: `path:line:col` (locations) or
// `kind Name (in Container) — path:line:col` (symbols), or a "No X found."
// sentence. Symbol lines split on the em-dash so the location reads as metadata
// next to the symbol.
function LspLocationsPreview({ tool, onOpenView }: ToolPreviewProps) {
  const rows = resultLines(tool.result);
  return (
    <div className={PREVIEW_WRAP}>
      {rows.length === 0 && (
        <PreviewPlaceholder status={tool.status} pending="Querying…" idle="(empty)" />
      )}
      {rows.slice(0, MAX_ROWS).map((row, i) => {
        const sep = row.lastIndexOf(" — ");
        if (sep === -1) {
          return (
            <div
              key={i}
              className="truncate rounded-[4px] px-1 py-0.5 text-fg-soft hover:bg-fg/[0.04]"
            >
              {row}
            </div>
          );
        }
        return (
          <div
            key={i}
            className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 rounded-[4px] px-1 py-0.5 hover:bg-fg/[0.04]"
          >
            <span className="truncate text-fg-soft">{row.slice(0, sep)}</span>
            <span className="truncate text-[11px] text-fg-muted">{row.slice(sep + 3)}</span>
          </div>
        );
      })}
      <Overflow count={rows.length - MAX_ROWS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

function LspHoverPreview({ tool, onOpenView }: ToolPreviewProps) {
  const text = tool.result?.trim();
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre-wrap break-words text-fg-soft")}>
      {text || <PreviewPlaceholder status={tool.status} pending="Querying…" idle="(empty)" />}
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
    <div className={PREVIEW_WRAP}>
      {rows.slice(0, MAX_ROWS).map((row, i) => {
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
      <Overflow count={rows.length - MAX_ROWS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// Pick the hover renderer for hover, locations for every other operation;
// default to locations when the operation isn't visible (args are suppressed
// once the call has a label — see projections.argsText).
function LspPreview(props: ToolPreviewProps) {
  return lspPreviewOperation(props.tool.args) === "hover" ? (
    <LspHoverPreview {...props} />
  ) : (
    <LspLocationsPreview {...props} />
  );
}

export const lspPreviews = definePlugin({
  name: "lyra.builtin.lsp-previews",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, LspPreview, { key: "lsp" });
    host.extensions.contribute(TOOL_PREVIEW, LspDiagnosticsPreview, { key: "lsp_diagnostics" });
  },
});
