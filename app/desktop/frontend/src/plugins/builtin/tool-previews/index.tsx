// Built-in plugins: inline previews for the standard tool calls.
//
// Each preview is a small React component plus a `host.extensions.contribute(TOOL_PREVIEW, …)`
// call. They go through the same SDK surface third-party plugins use, so
// adding a new tool fn means writing a similar plugin — no special-casing.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { useDiff, useFileHead, useGrep, useTerminal } from "@/lib/data/queries";
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

function BashPreview({ onOpenView }: ToolPreviewProps) {
  const { data: lines } = useTerminal();
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre text-fg-soft")}>
      {(lines ?? []).slice(0, MAX_TERM_LINES).map((l, i) => (
        <span key={i} className={l.kind}>
          {l.text}
        </span>
      ))}
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
          const tone =
            row.type === "add"
              ? "bg-[rgba(30,215,96,0.07)]"
              : row.type === "del"
                ? "bg-[rgba(243,114,127,0.07)]"
                : "";
          const meta =
            row.type === "add"
              ? "text-[rgba(95,227,154,0.7)]"
              : row.type === "del"
                ? "text-[rgba(243,114,127,0.7)]"
                : "text-fg-faint";
          const codeTone =
            row.type === "add"
              ? "text-[#c8f5d8]"
              : row.type === "del"
                ? "text-[#f5cdd2]"
                : "text-fg-soft";
          const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
          return (
            <div key={i} className={cn("grid grid-cols-[18px_1fr] px-0.5", tone)}>
              <span className={cn("text-center text-[11px] select-none", meta)}>{sign}</span>
              <span className={cn("whitespace-pre", codeTone)}>{row.code}</span>
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

export const bash = definePlugin({
  name: "lyra.builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "bash" });
  },
});

// One plugin covers both file-write tool kinds — they share the diff renderer.
export const diff = definePlugin({
  name: "lyra.builtin.diff",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, DiffPreview, { key: "edit_file" });
    host.extensions.contribute(TOOL_PREVIEW, DiffPreview, { key: "write_file" });
  },
});

export const file = definePlugin({
  name: "lyra.builtin.file",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, FilePreview, { key: "read_file" });
  },
});

export const grep = definePlugin({
  name: "lyra.builtin.grep",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, GrepPreview, { key: "grep" });
  },
});
