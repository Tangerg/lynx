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
  // output reconciled from the completed Item's commandExecution.output, with
  // the toolOutput delta stream as the live preview while running (see
  // projections.ts + docs/protocol/TOOL_OUTPUT.md).
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

function FilePreview({ onOpenView }: ToolPreviewProps) {
  const { data: lines } = useFileHead();
  return (
    <div className={PREVIEW_WRAP}>
      {/* file-preview class kept as a hook so highlight.ts spans
          (.t-kw/.t-str/.t-fn) emitted via dangerouslySetInnerHTML can be
          coloured by tool.css — those classes come from a string, not
          JSX, so Tailwind utilities aren't applicable. */}
      <div className="file-preview font-mono text-[11.5px] leading-[1.55]">
        {(lines ?? []).map((l, i) => (
          <div
            key={i}
            className={cn("fp-line grid grid-cols-[28px_1fr] gap-2.5", l.muted && "text-fg-faint")}
          >
            <span className="text-right text-[11px] text-fg-faint select-none">{l.ln}</span>
            <span
              className={cn("code", l.muted && "text-fg-faint")}
              dangerouslySetInnerHTML={{ __html: l.code }}
            />
          </div>
        ))}
      </div>
      <PreviewFoot label="View full file" onClick={onOpenView} />
    </div>
  );
}

function GrepPreview({ onOpenView }: ToolPreviewProps) {
  const { data } = useGrep();
  const matches = data?.matches ?? [];
  const total = data?.total ?? matches.length;
  const visible = matches.slice(0, MAX_GREP_MATCHES);
  const overflow = total - visible.length;
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {visible.map((m, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 py-0.5 whitespace-nowrap overflow-hidden"
          >
            <span className="truncate text-[11px] text-fg-faint">{m.path}</span>
            <span className="truncate text-fg">{m.match}</span>
          </div>
        ))}
        {overflow > 0 && <div className="pt-1 text-fg-faint">… {overflow} more matches</div>}
      </div>
      <PreviewFoot label="View all matches" onClick={onOpenView} />
    </div>
  );
}

// Previews are keyed by the tool ROUTING KEY (see toolRoutingKey): typed
// variants use their ToolInvocation.kind; the generic `tool` envelope uses
// the tool name.
export const bash = definePlugin({
  name: "lyra.builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "commandExecution" });
  },
});

export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, DiffPreview, { key: "fileChange" });
  },
});

export const file = definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    // generic `read` tool (by name)
    host.extensions.contribute(TOOL_PREVIEW, FilePreview, { key: "read" });
    host.extensions.contribute(TOOL_PREVIEW, FilePreview, { key: "read_file" });
  },
});

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    // local grep/glob → kind "search"
    host.extensions.contribute(TOOL_PREVIEW, GrepPreview, { key: "search" });
  },
});
