import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useWorkspaceFileHead, useWorkspaceGrep } from "@/plugins/builtin/workspace/public/queries";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { parseJsonResult } from "./toolResultParsing";

export function useFileToolPreview(tool: ToolCall, maxLines: number) {
  const workspace = useActiveSessionWorkspace();
  const path = tool.fn && tool.fn !== tool.name ? tool.fn : undefined;
  return useWorkspaceFileHead(
    path && workspace.status === "ready"
      ? { path, cwd: workspace.cwd, lines: maxLines }
      : undefined,
  );
}

interface GrepPreviewRow {
  loc: string;
  text: string;
}

// The runtime projects every grep shape — matches, file names, per-file counts — into
// one `hits: [{path, snippet?, lineNumber?}]` envelope, so a call's own rows come from
// that and nothing has to guess which output mode produced them.
function inlineGrepRows(result: string | undefined): GrepPreviewRow[] | undefined {
  const hits = parseJsonResult(result)?.hits;
  if (!Array.isArray(hits)) return undefined;
  return hits.map((hit) => {
    const record = typeof hit === "object" && hit !== null ? (hit as Record<string, unknown>) : {};
    const path = String(record.path ?? "");
    const line = record.lineNumber;
    return {
      loc: typeof line === "number" ? `${path}:${line}` : path,
      text: String(record.snippet ?? ""),
    };
  });
}

export function useGrepToolPreview(tool: ToolCall, maxMatches: number) {
  const inline = inlineGrepRows(tool.result);
  const workspace = useActiveSessionWorkspace();
  const query =
    !inline && tool.name === "grep" && tool.fn && tool.fn !== "search" ? tool.fn : undefined;
  const { data } = useWorkspaceGrep(
    query && workspace.status === "ready"
      ? { query, cwd: workspace.cwd, limit: maxMatches }
      : undefined,
  );
  const rows =
    inline ??
    (data?.matches ?? []).map((match) => ({
      loc: `${match.path}:${match.lineNumber}`,
      text: match.text,
    }));
  const shown = rows.slice(0, maxMatches);
  return {
    shown,
    overflow: inline ? rows.length - shown.length : (data?.total ?? 0) - shown.length,
  };
}
