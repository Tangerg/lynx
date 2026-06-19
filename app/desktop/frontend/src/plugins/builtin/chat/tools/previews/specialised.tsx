// Built-in plugins: inline previews for the runtime's specialised tools —
// lsp / lsp_diagnostics (code intelligence), skill, task (sub-agent), ask_user,
// glob, web_search. Each renders the tool call's OWN result (these tools return
// their data inline, no aux-API re-query needed). Same registration surface as
// the previews in index.tsx.

import type { SearchResult } from "@/components/tools/previews/SearchResults";
import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/components/tools/previews/PreviewPlaceholder";
import { SearchResults } from "@/components/tools/previews/SearchResults";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { parseJsonResult, PREVIEW_WRAP, resultLines } from "./shared";

const MAX_ROWS = 9;
const MAX_WEB_RESULTS = 8;

function Overflow({ count }: { count: number }) {
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {count} more</div>;
}

// ---- lsp (operation-dispatched) ----
//
// Result is one line per hit: `path:line:col` (locations) or
// `kind Name (in Container) — path:line:col` (symbols), or a
// "No X found." sentence. Symbol lines split on the em-dash so the
// location reads as metadata next to the symbol.
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
            <div key={i} className="truncate py-0.5 text-fg-soft">
              {row}
            </div>
          );
        }
        return (
          <div key={i} className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 py-0.5">
            <span className="truncate text-fg">{row.slice(0, sep)}</span>
            <span className="truncate text-[11px] text-fg-faint">{row.slice(sep + 3)}</span>
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

//
// `severity path:line:col: message [source]` per line — tint the severity
// word so a wall of diagnostics scans by color.
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

//
// op="list" returns an <available_skills> XML catalog — parse it into
// name + description rows. op="load" / "load_resource" return the skill's
// markdown / resource text: show the head.
const SKILL_ENTRY = /<skill>\s*<name>([\s\S]*?)<\/name>\s*<description>([\s\S]*?)<\/description>/g;

function SkillPreview({ tool, onOpenView }: ToolPreviewProps) {
  const text = tool.result ?? "";
  const entries = [...text.matchAll(SKILL_ENTRY)].map((m) => ({
    name: m[1]!.trim(),
    description: m[2]!.trim(),
  }));
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

//
// The result is the sub-agent's final reply. The child run itself streams
// on the same tree (spawnedByItemId) — this preview is the parent-side
// summary of what came back.
function TaskPreview({ tool, onOpenView }: ToolPreviewProps) {
  const lines = resultLines(tool.result);
  return (
    <div className={PREVIEW_WRAP}>
      <div className="whitespace-pre-wrap break-words text-fg-soft">
        {lines.slice(0, MAX_ROWS).join("\n") ||
          (tool.status === "running" ? "Sub-agent working…" : "(no reply)")}
      </div>
      <Overflow count={lines.length - MAX_ROWS} />
      <PreviewFoot label="tools.preview.viewReply" onClick={onOpenView} />
    </div>
  );
}

// The answer may arrive as plain text, `{ answer }`, or the wire answers map
// (QuestionField.name → string[]) — flatten any of them to one readable line.
// (ask_user usually renders as the interactive QuestionCard, not here; this is
// the fallback for a runtime that returns it as a plain tool result instead.)
function askUserAnswer(result: string | undefined): string {
  const text = result?.trim();
  if (!text) return "";
  const parsed = parseJsonResult(result);
  if (!parsed) return text; // plain-text answer
  const direct = parsed.answer ?? parsed.response;
  if (typeof direct === "string") return direct;
  const parts = Object.values(parsed).map((v) =>
    typeof v === "string"
      ? v
      : Array.isArray(v)
        ? v.filter((x) => typeof x === "string").join(", ")
        : "",
  );
  return parts.filter(Boolean).join(" · ") || text;
}

//
// The question rides the card title (fn); the result is the human's answer
// once the HITL interrupt resolves.
function AskUserPreview({ tool }: ToolPreviewProps) {
  const answer = askUserAnswer(tool.result);
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre-wrap break-words")}>
      {answer ? (
        <>
          <span className="text-fg-faint">answer · </span>
          <span className="text-fg-soft">{answer}</span>
        </>
      ) : (
        <span className="text-fg-faint">Waiting for your answer…</span>
      )}
    </div>
  );
}

//
// glob's result is the §4.4.2 search shape `{ hits: SearchHit[] }` (each hit's
// `.path`), but tolerate a raw `paths` / `files` string list and `matches` too
// — the SAME key priority the projection counts for the header badge, so the
// body list can never disagree with the "N matches" count (the bug where 20
// matches rendered "(no matches)" was reading only `paths`).
function globPaths(parsed: Record<string, unknown> | undefined): string[] {
  const arr = [parsed?.hits, parsed?.matches, parsed?.files, parsed?.paths].find(Array.isArray);
  if (!arr) return [];
  return (arr as unknown[]).map(hitPath).filter((p) => p.length > 0);
}

function hitPath(hit: unknown): string {
  if (typeof hit === "string") return hit;
  if (typeof hit === "object" && hit !== null)
    return String((hit as Record<string, unknown>).path ?? "");
  return "";
}

function GlobPreview({ tool, onOpenView }: ToolPreviewProps) {
  const parsed = parseJsonResult(tool.result);
  const paths = globPaths(parsed);
  return (
    <div className={PREVIEW_WRAP}>
      {paths.length === 0 && (
        <PreviewPlaceholder status={tool.status} pending="Matching…" idle="(no matches)" />
      )}
      {paths.slice(0, MAX_ROWS).map((p) => (
        <div key={p} className="truncate py-0.5 text-fg-soft">
          {p}
        </div>
      ))}
      <Overflow count={paths.length - MAX_ROWS} />
      {parsed?.truncated === true && <div className="text-fg-faint">… truncated by runtime</div>}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// The runtime exposes ONE `lsp` tool (operation in the args) plus a separate
// `lsp_diagnostics`. Pick the hover renderer for hover, locations for every
// other operation; default to locations when the operation isn't visible (args
// are suppressed once the call has a label — see projections.argsText).
function LspPreview(props: ToolPreviewProps) {
  let op = "";
  try {
    op = String((JSON.parse(props.tool.args || "{}") as Record<string, unknown>).operation ?? "");
  } catch {
    /* partial / empty args — fall through to the locations renderer */
  }
  return op === "hover" ? <LspHoverPreview {...props} /> : <LspLocationsPreview {...props} />;
}

export const lspPreviews = definePlugin({
  name: "lyra.builtin.lsp-previews",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, LspPreview, { key: "lsp" });
    host.extensions.contribute(TOOL_PREVIEW, LspDiagnosticsPreview, { key: "lsp_diagnostics" });
  },
});

export const skillPreview = definePlugin({
  name: "lyra.builtin.skill-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, SkillPreview, { key: "skill" });
  },
});

export const taskPreview = definePlugin({
  name: "lyra.builtin.task-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, TaskPreview, { key: "task" });
    host.extensions.contribute(TOOL_PREVIEW, TaskPreview, { key: "subagent" });
  },
});

export const askUserPreview = definePlugin({
  name: "lyra.builtin.ask-user-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, AskUserPreview, { key: "ask_user" });
  },
});

export const globPreview = definePlugin({
  name: "lyra.builtin.glob-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, GlobPreview, { key: "glob" });
  },
});

// web_search returns the §4.4.2 shape `{ results: WebSearchResult[] }`
// ({ title, url, snippet, faviconUrl }); we project each to a SearchResult card
// (domain derived from the url) and render the shared grid. The favicon-letter
// badge keeps it offline-friendly — no external favicon fetch.
function domainOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function webSearchResults(result: string | undefined): SearchResult[] {
  const arr = parseJsonResult(result)?.results;
  if (!Array.isArray(arr)) return [];
  return arr.flatMap((r) => {
    const o = typeof r === "object" && r !== null ? (r as Record<string, unknown>) : undefined;
    const url = typeof o?.url === "string" ? o.url : "";
    if (!url) return [];
    return [
      {
        url,
        domain: domainOf(url),
        title: typeof o?.title === "string" && o.title ? o.title : url,
        snippet: typeof o?.snippet === "string" ? o.snippet : "",
      },
    ];
  });
}

function WebSearchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const results = webSearchResults(tool.result);
  if (results.length === 0) {
    return (
      <div className={PREVIEW_WRAP}>
        <PreviewPlaceholder status={tool.status} pending="Searching…" idle="(no results)" />
      </div>
    );
  }
  return (
    <div className="bg-canvas px-3.5 pt-2.5 pb-2">
      <SearchResults results={results.slice(0, MAX_WEB_RESULTS)} />
      <Overflow count={results.length - MAX_WEB_RESULTS} />
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

export const webSearchPreview = definePlugin({
  name: "lyra.builtin.web-search-preview",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, WebSearchPreview, { key: "web_search" });
  },
});
