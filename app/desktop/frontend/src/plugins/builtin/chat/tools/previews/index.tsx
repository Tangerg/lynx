// Built-in plugins: inline previews for the standard tool calls.
//
// Each preview is a small React component plus a `host.extensions.contribute(TOOL_PREVIEW, …)`
// call. They go through the same SDK surface third-party plugins use, so
// adding a new tool fn means writing a similar plugin — no special-casing.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/components/tools/previews/PreviewPlaceholder";
import { useDiff, useFileHead, useGrep } from "@/lib/data/queries";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { useServerFeature } from "@/state/runtimeStore";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { parseJsonResult, PREVIEW_WRAP } from "./shared";

export { askUserPreview, globPreview, lspPreviews, skillPreview, taskPreview } from "./specialised";

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

function DiffPreview({ onOpenView }: ToolPreviewProps) {
  // Whole-worktree diff of the active session's cwd (the tool ran there);
  // never called without git.
  const gitEnabled = useServerFeature("git");
  const cwd = useActiveSessionCwd();
  const { data } = useDiff(gitEnabled ? { cwd } : undefined);
  // Flatten per-file diffs for the glance view — each file's path becomes a
  // hunk-style separator row so MAX_DIFF_ROWS stays one simple slice.
  const rows = (data?.files ?? []).flatMap((f) => [
    { type: "hunk" as const, text: f.path },
    ...f.rows,
  ]);
  const hiddenRows = rows.length - MAX_DIFF_ROWS;
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
        {(hiddenRows > 0 || data?.truncated) && (
          <div className="text-fg-faint">
            {hiddenRows > 0 && `… ${hiddenRows} more rows`}
            {data?.truncated && " · truncated by runtime"}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.openDiff" onClick={onOpenView} />
    </div>
  );
}

// Both parameterized previews read their query off `tool.fn` — the §4.4.2
// projection bakes the key argument into the display name (read → path,
// search → query; see core-reducer toolLabel). A started shell without args
// still shows the projection fallback (the tool name / "search"), so treat
// that as "nothing to ask yet" and let the hook stay disabled.
function FilePreview({ tool, onOpenView }: ToolPreviewProps) {
  // cwd = the active session's workspace — the tool ran there, so the
  // preview must read the same tree (the serve dir may be elsewhere).
  const cwd = useActiveSessionCwd();
  const path = tool.fn && tool.fn !== tool.name ? tool.fn : undefined;
  const { data: lines } = useFileHead(path ? { path, cwd, lines: MAX_FILE_LINES } : undefined);
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

// One display row per hit, whatever grep's output_mode returned — matches
// ({path,line,text}), files (paths), or counts ({path,count}). Undefined
// when the result isn't one of those shapes (still streaming / legacy
// `hits` convention) — the caller then falls back to re-querying.
function inlineGrepRows(
  result: string | undefined,
): { rows: { loc: string; text: string }[]; truncated: boolean } | undefined {
  const parsed = parseJsonResult(result);
  if (!parsed) return undefined;
  const rec = (v: unknown): Record<string, unknown> =>
    typeof v === "object" && v !== null ? (v as Record<string, unknown>) : {};
  const truncated = parsed.truncated === true;
  if (Array.isArray(parsed.matches)) {
    return {
      rows: parsed.matches.map((m) => ({
        loc: `${String(rec(m).path ?? "")}:${String(rec(m).line ?? "")}`,
        text: String(rec(m).text ?? ""),
      })),
      truncated,
    };
  }
  if (Array.isArray(parsed.files)) {
    return { rows: parsed.files.map((f) => ({ loc: String(f), text: "" })), truncated };
  }
  if (Array.isArray(parsed.counts)) {
    return {
      rows: parsed.counts.map((c) => ({
        loc: String(rec(c).path ?? ""),
        text: `${String(rec(c).count ?? 0)} matches`,
      })),
      truncated,
    };
  }
  return undefined;
}

function GrepPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Prefer the call's OWN result — grep returns matches/files/counts inline
  // (output_mode), which also honors filters (glob/type/context) a re-query
  // can't reproduce. The workspace.grep re-query stays as the fallback for
  // runtimes whose result carries only the §4.4.2 `hits` convention.
  const inline = inlineGrepRows(tool.result);
  const cwd = useActiveSessionCwd();
  const query =
    !inline && tool.name === "grep" && tool.fn && tool.fn !== "search" ? tool.fn : undefined;
  const { data } = useGrep(query ? { query, cwd, limit: MAX_GREP_MATCHES } : undefined);
  const rows =
    inline?.rows ??
    (data?.matches ?? []).map((m) => ({ loc: `${m.path}:${m.lineNumber}`, text: m.text }));
  const shown = rows.slice(0, MAX_GREP_MATCHES);
  // §7.5 no-silent-caps: surface both our preview cap and server truncation.
  const overflow = inline ? rows.length - shown.length : (data?.total ?? 0) - shown.length;
  return (
    <div className={PREVIEW_WRAP}>
      <div className="font-mono text-[11.5px] leading-[1.55]">
        {shown.map((r, i) => (
          <div
            key={i}
            className="grid grid-cols-[200px_1fr] gap-3 py-0.5 whitespace-nowrap overflow-hidden"
          >
            <span className="truncate text-[11px] text-fg-faint">{r.loc}</span>
            <span className="truncate text-fg">{r.text}</span>
          </div>
        ))}
        {overflow > 0 && <div className="pt-1 text-fg-faint">… {overflow} more matches</div>}
        {inline?.truncated && <div className="pt-1 text-fg-faint">… truncated by runtime</div>}
      </div>
      <PreviewFoot label="tools.preview.viewMatches" onClick={onOpenView} />
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
    // Background-shell family: all three return terminal-style plain text
    // (start ack / incremental output + status line / kill confirmation).
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "run_in_background" });
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "bash_output" });
    host.extensions.contribute(TOOL_PREVIEW, BashPreview, { key: "kill_shell" });
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
