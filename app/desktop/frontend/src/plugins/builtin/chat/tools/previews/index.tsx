// Built-in plugins: inline previews for the standard tool calls.
//
// Each preview is a small React component plus a `host.extensions.contribute(TOOL_PREVIEW, …)`
// call. They go through the same SDK surface third-party plugins use, so
// adding a new tool fn means writing a similar plugin — no special-casing.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import {
  useDiffToolPreview,
  useFileToolPreview,
  useGrepToolPreview,
} from "@/plugins/builtin/chat/tools/application/toolPreviewData";
import { PREVIEW_WRAP } from "./shared";

// Specialised previews — one file per tool family. Re-exported here so the
// manifest imports the whole preview set from a single module.
export { askUserPreview } from "./askUser";
export { globPreview } from "./glob";
export { lspPreviews } from "./lsp";
export { skillPreview } from "./skill";
export { taskPreview } from "./task";
export { webSearchPreview } from "./webSearch";

const MAX_TERM_LINES = 9;
const MAX_DIFF_ROWS = 8;
const MAX_GREP_MATCHES = 4;
const MAX_FILE_LINES = 40;

// Per-line presentation keyed by row type — one lookup beats four parallel
// ternary chains switching on the same field. (The full DiffView in the
// workspace plugin keeps its own narrower table: it highlights code via shiki
// instead of carrying a flat `codeTone`, so the two don't share a module.)
const ROW_STYLE: Record<
  "added" | "deleted" | "context",
  { tone: string; meta: string; codeTone: string; sign: string }
> = {
  added: {
    tone: "bg-[rgba(30,215,96,0.07)]",
    meta: "text-[rgba(95,227,154,0.7)]",
    codeTone: "text-[#c8f5d8]",
    sign: "+",
  },
  deleted: {
    tone: "bg-[rgba(243,114,127,0.07)]",
    meta: "text-[rgba(243,114,127,0.7)]",
    codeTone: "text-[#f5cdd2]",
    sign: "−",
  },
  context: { tone: "", meta: "text-fg-faint", codeTone: "text-fg-soft", sign: " " },
};

function ShellPreview({ tool, onOpenView }: ToolPreviewProps) {
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
        lines.slice(0, MAX_TERM_LINES).map((text, i) => (
          <div key={i}>
            <LinkedText text={text || " "} />
          </div>
        ))
      ) : (
        <PreviewPlaceholder status={tool.status} pending="Running…" idle="(no output)" />
      )}
      {(hiddenLines > 0 || tool.outputTruncated) && (
        <div className="text-fg-faint">
          {hiddenLines > 0 && `… ${hiddenLines} more lines`}
          {tool.outputTruncated && " · output truncated by runtime"}
        </div>
      )}
      <PreviewFoot label="tools.preview.openTerminal" onClick={onOpenView} />
    </div>
  );
}

function DiffPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Prefer THIS edit's call-scoped patch (FileEdit.diff, §12.1 C) — exactly
  // what the edit changed. Fall back to the whole-worktree diff for a `write`
  // (no call-scoped diff) or until the completed item carries one; each file's
  // path becomes a hunk-style separator row so MAX_DIFF_ROWS stays one slice.
  const { rows, truncated, hiddenRows } = useDiffToolPreview(tool, MAX_DIFF_ROWS);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {rows.slice(0, MAX_DIFF_ROWS).map((row, i) => {
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
        {(hiddenRows > 0 || truncated) && (
          <div className="text-fg-faint">
            {hiddenRows > 0 && `… ${hiddenRows} more rows`}
            {truncated && " · truncated by runtime"}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

// Both parameterized previews read their query off `tool.fn` — the §4.4.2
// projection bakes the key argument into the display name (read → path,
// search → query; see agent fold toolLabel). A started shell without args
// still shows the projection fallback (the tool name / "search"), so treat
// that as "nothing to ask yet" and let the hook stay disabled.
function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  // cwd = the active session's workspace — the tool ran there, so the
  // preview must read the same tree (the serve dir may be elsewhere).
  const { data: lines } = useFileToolPreview(tool, MAX_FILE_LINES);
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
      <PreviewFoot label="tools.preview.viewFile" onClick={onOpenView} />
    </div>
  );
}

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Prefer the call's OWN result — grep returns matches/files/counts inline
  // (output_mode), which also honors filters (glob/type/context) a re-query
  // can't reproduce. The workspace.grep re-query stays as the fallback for
  // runtimes whose result carries only the §4.4.2 `hits` convention.
  const { shown, overflow, truncated } = useGrepToolPreview(tool, MAX_GREP_MATCHES);
  // §7.5 no-silent-caps: surface both our preview cap and server truncation.
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {shown.map((r, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 py-0.5 whitespace-nowrap overflow-hidden"
          >
            <span className="truncate text-[11px] text-fg-faint">
              <LinkedText text={r.loc} />
            </span>
            <span className="truncate text-fg">{r.text}</span>
          </div>
        ))}
        {overflow > 0 && <div className="pt-1 text-fg-faint">… {overflow} more matches</div>}
        {truncated && <div className="pt-1 text-fg-faint">… truncated by runtime</div>}
      </div>
      <PreviewFoot label="tools.preview.viewMatches" onClick={onOpenView} />
    </div>
  );
}

// Previews are keyed by the tool ROUTING KEY = the wire tool `name` (§4.4 /
// §4.4.2 display conventions).
export const shellPreview = definePlugin({
  name: "lyra.builtin.shell",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell" });
    // Background-shell family: all three return terminal-style plain text
    // (start ack / incremental output + status line / kill confirmation).
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "run_in_background" });
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell_output" });
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell_kill" });
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
    // glob gets its own preview (specialised.tsx) — a glob pattern is not a
    // workspace.grep query, and GlobResponse carries the paths inline.
    host.extensions.contribute(TOOL_PREVIEW, GrepPreview, { key: "grep" });
  },
});
