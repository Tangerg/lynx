// Built-in plugins: inline previews for the standard tool calls.
//
// Each preview is a small React component plus a `host.extensions.contribute(TOOL_PREVIEW, …)`
// call. They go through the same SDK surface third-party plugins use, so
// adding a new tool fn means writing a similar plugin — no special-casing.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useDiff, useFileHead, useGrep } from "@/lib/data/queries";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";

const MAX_TERM_LINES = 9;
const MAX_DIFF_ROWS = 8;
const MAX_GREP_MATCHES = 4;
const MAX_FILE_LINES = 40;

// Shared container shape for every inline tool preview. The wrapper sits
// one step deeper than the .tool-card surface (canvas) so it reads as
// nested content — DESIGN.md §5 surface-step depth rule.
const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto bg-canvas px-3.5 pt-2.5 pb-2 font-mono text-[12px] leading-[1.55] text-fg-muted";

// Per-line presentation keyed by row type — one lookup beats four parallel
// ternary chains switching on the same field. (The full DiffView in the
// workspace plugin keeps its own narrower table: it highlights code via shiki
// instead of carrying a flat `codeTone`, so the two don't share a module.)
const ROW_STYLE: Record<
  "add" | "del" | "ctx",
  { tone: string; meta: string; codeTone: string; sign: string }
> = {
  add: {
    tone: "bg-[rgba(30,215,96,0.07)]",
    meta: "text-[rgba(95,227,154,0.7)]",
    codeTone: "text-[#c8f5d8]",
    sign: "+",
  },
  del: {
    tone: "bg-[rgba(243,114,127,0.07)]",
    meta: "text-[rgba(243,114,127,0.7)]",
    codeTone: "text-[#f5cdd2]",
    sign: "−",
  },
  ctx: { tone: "", meta: "text-fg-faint", codeTone: "text-fg-soft", sign: " " },
};

function BashPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Render THIS call's stdout from `tool.result` — the authoritative merged
  // output reconciled from the completed Item's tool.result.output, with
  // the toolOutput delta stream as the live preview while running (see
  // projections.ts + API.md §4.4.1).
  const output = tool.result?.replace(/\n+$/, "");
  const lines = output ? output.split("\n") : [];
  const hiddenLines = lines.length - MAX_TERM_LINES;
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre-wrap break-all text-fg-soft")}>
      {lines.length > 0 ? (
        lines.slice(0, MAX_TERM_LINES).map((text, i) => <div key={i}>{text || " "}</div>)
      ) : (
        <div className="text-fg-faint">
          {tool.status === "running" ? "Running…" : "(no output)"}
        </div>
      )}
      {(hiddenLines > 0 || tool.outputTruncated) && (
        <div className="text-fg-faint">
          {hiddenLines > 0 && `… ${hiddenLines} more lines`}
          {tool.outputTruncated && " · output truncated by runtime"}
        </div>
      )}
      <PreviewFoot label="Open in Terminal" onClick={onOpenView} />
    </div>
  );
}

function DiffPreview({ onOpenView }: ToolPreviewProps) {
  const { data: rows } = useDiff();
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {(rows ?? []).slice(0, MAX_DIFF_ROWS).map((row, i) => {
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
      </div>
      <PreviewFoot label="Open full diff" onClick={onOpenView} />
    </div>
  );
}

// Both parameterized previews read their query off `tool.fn` — the §4.4.2
// projection bakes the key argument into the display name (read → path,
// search → query; see core-reducer toolLabel). A started shell without args
// still shows the projection fallback (the tool name / "search"), so treat
// that as "nothing to ask yet" and let the hook stay disabled.
function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  const path = tool.fn && tool.fn !== tool.name ? tool.fn : undefined;
  const { data: lines } = useFileHead(path ? { path, lines: MAX_FILE_LINES } : undefined);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {(lines ?? []).map((l) => (
          <div key={l.lineNumber} className="grid grid-cols-[28px_1fr] gap-2.5">
            <span className="text-right text-[11px] text-fg-faint select-none">{l.lineNumber}</span>
            <span className="whitespace-pre text-fg-soft">{l.text || " "}</span>
          </div>
        ))}
      </div>
      <PreviewFoot label="View full file" onClick={onOpenView} />
    </div>
  );
}

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Re-querying makes sense only for regex search — a glob pattern is not a
  // workspace.grep query, so the glob preview keeps just the footer link.
  const query = tool.name === "grep" && tool.fn && tool.fn !== "search" ? tool.fn : undefined;
  const { data } = useGrep(query ? { query, limit: MAX_GREP_MATCHES } : undefined);
  const matches = data?.matches ?? [];
  // §7.5 no-silent-caps: total may exceed matches.length (server truncation).
  const overflow = (data?.total ?? 0) - matches.length;
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {matches.map((m, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 py-0.5 whitespace-nowrap overflow-hidden"
          >
            <span className="truncate text-[11px] text-fg-faint">
              {m.path}:{m.lineNumber}
            </span>
            <span className="truncate text-fg">{m.text}</span>
          </div>
        ))}
        {overflow > 0 && <div className="pt-1 text-fg-faint">… {overflow} more matches</div>}
      </div>
      <PreviewFoot label="View all matches" onClick={onOpenView} />
    </div>
  );
}

// Previews are keyed by the tool ROUTING KEY = the wire tool `name` (§4.4 /
// §4.4.2 display conventions).
export const bash = definePlugin({
  name: "lyra.builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "bash" });
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "shell" });
  },
});

export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, DiffPreview, { key: "edit" });
    host.extensions.contribute(TOOL_PREVIEW, DiffPreview, { key: "write" });
  },
});

export const file = definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, FilePreview, { key: "read" });
  },
});

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, GrepPreview, { key: "grep" });
    host.extensions.contribute(TOOL_PREVIEW, GrepPreview, { key: "glob" });
  },
});
