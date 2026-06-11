// Built-in plugins: inline previews for the runtime's specialised tools —
// lsp_* (code intelligence), skill, task (sub-agent), ask_user, glob.
// Each renders the tool call's OWN result (these tools return their data
// inline, no aux-API re-query needed). Same registration surface as the
// previews in index.tsx.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { parseJsonResult, PREVIEW_WRAP, resultLines } from "./shared";

const MAX_ROWS = 9;

function Overflow({ count }: { count: number }) {
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {count} more</div>;
}

// ---- lsp_definition / lsp_references / lsp_document_symbols /
// ---- lsp_workspace_symbols -------------------------------------------------
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
        <div className="text-fg-faint">{tool.status === "running" ? "Querying…" : "(empty)"}</div>
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
      <PreviewFoot label="View details" onClick={onOpenView} />
    </div>
  );
}

// ---- lsp_hover --------------------------------------------------------------
function LspHoverPreview({ tool, onOpenView }: ToolPreviewProps) {
  const text = tool.result?.trim();
  return (
    <div className={cn(PREVIEW_WRAP, "whitespace-pre-wrap break-words text-fg-soft")}>
      {text || (
        <span className="text-fg-faint">{tool.status === "running" ? "Querying…" : "(empty)"}</span>
      )}
      <PreviewFoot label="View details" onClick={onOpenView} />
    </div>
  );
}

// ---- lsp_diagnostics --------------------------------------------------------
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
      <PreviewFoot label="View details" onClick={onOpenView} />
    </div>
  );
}

// ---- skill ------------------------------------------------------------------
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
        <PreviewFoot label="View full text" onClick={onOpenView} />
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
      <PreviewFoot label="View details" onClick={onOpenView} />
    </div>
  );
}

// ---- task (sub-agent) -------------------------------------------------------
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
      <PreviewFoot label="View full reply" onClick={onOpenView} />
    </div>
  );
}

// ---- ask_user ---------------------------------------------------------------
//
// The question rides the card title (fn); the result is the human's answer
// once the HITL interrupt resolves.
function AskUserPreview({ tool }: ToolPreviewProps) {
  const answer = tool.result?.trim();
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

// ---- glob -------------------------------------------------------------------
//
// GlobResponse { paths, truncated? } — a path list, rendered from the
// call's own result (glob patterns aren't workspace.grep queries, so
// there's nothing to re-query).
function GlobPreview({ tool, onOpenView }: ToolPreviewProps) {
  const parsed = parseJsonResult(tool.result);
  const paths = Array.isArray(parsed?.paths) ? parsed.paths.filter(isString) : [];
  return (
    <div className={PREVIEW_WRAP}>
      {paths.length === 0 && (
        <div className="text-fg-faint">
          {tool.status === "running" ? "Matching…" : "(no matches)"}
        </div>
      )}
      {paths.slice(0, MAX_ROWS).map((p) => (
        <div key={p} className="truncate py-0.5 text-fg-soft">
          {p}
        </div>
      ))}
      <Overflow count={paths.length - MAX_ROWS} />
      {parsed?.truncated === true && <div className="text-fg-faint">… truncated by runtime</div>}
      <PreviewFoot label="View details" onClick={onOpenView} />
    </div>
  );
}

function isString(v: unknown): v is string {
  return typeof v === "string";
}

// ---- registrations ----------------------------------------------------------

export const lspPreviews = definePlugin({
  name: "lyra.builtin.lsp-previews",
  version: "1.0.0",
  setup({ host }) {
    for (const key of [
      "lsp_definition",
      "lsp_references",
      "lsp_document_symbols",
      "lsp_workspace_symbols",
    ]) {
      host.extensions.contribute(TOOL_PREVIEW, LspLocationsPreview, { key });
    }
    host.extensions.contribute(TOOL_PREVIEW, LspHoverPreview, { key: "lsp_hover" });
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
