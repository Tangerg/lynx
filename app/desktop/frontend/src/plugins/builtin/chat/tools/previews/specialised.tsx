import type { ToolPreviewProps } from "@/plugins/sdk";
import { PreviewFoot } from "@/components/tools/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/components/tools/previews/PreviewPlaceholder";
import { SearchResults } from "@/components/tools/previews/SearchResults";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import {
  askUserPreviewAnswer,
  globPreviewData,
  lspPreviewOperation,
  skillPreviewEntries,
  webSearchPreviewResults,
} from "@/plugins/builtin/chat/tools/application/specialisedPreviewData";
import { resultLines } from "@/plugins/builtin/chat/tools/application/toolResultParsing";
import { PREVIEW_WRAP } from "./shared";

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

function SkillPreview({ tool, onOpenView }: ToolPreviewProps) {
  const entries = skillPreviewEntries(tool.result);
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

function AskUserPreview({ tool }: ToolPreviewProps) {
  const answer = askUserPreviewAnswer(tool.result);
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

function GlobPreview({ tool, onOpenView }: ToolPreviewProps) {
  const { paths, truncated } = globPreviewData(tool.result);
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
      {truncated && <div className="text-fg-faint">… truncated by runtime</div>}
      <PreviewFoot label="tools.preview.viewDetails" onClick={onOpenView} />
    </div>
  );
}

// The runtime exposes ONE `lsp` tool (operation in the args) plus a separate
// `lsp_diagnostics`. Pick the hover renderer for hover, locations for every
// other operation; default to locations when the operation isn't visible (args
// are suppressed once the call has a label — see projections.argsText).
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

function WebSearchPreview({ tool, onOpenView }: ToolPreviewProps) {
  const results = webSearchPreviewResults(tool.result);
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
