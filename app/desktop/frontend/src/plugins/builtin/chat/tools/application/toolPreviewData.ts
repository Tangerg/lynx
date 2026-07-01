import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { useDiff, useFileHead, useGrep } from "@/lib/data/queries";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";

export function useDiffToolPreview(tool: ToolCall, maxRows: number) {
  const gitEnabled = useRuntimeCapability("git");
  const cwd = useActiveSessionCwd();
  const { data } = useDiff(gitEnabled ? { cwd } : undefined);
  const rows = tool.diff
    ? tool.diff
    : (data?.files ?? []).flatMap((file) => [
        { type: "hunk" as const, text: file.path },
        ...file.rows,
      ]);
  return {
    rows,
    truncated: tool.diff ? false : Boolean(data?.truncated),
    hiddenRows: rows.length - maxRows,
  };
}

export function useFileToolPreview(tool: ToolCall, maxLines: number) {
  const cwd = useActiveSessionCwd();
  const path = tool.fn && tool.fn !== tool.name ? tool.fn : undefined;
  return useFileHead(path ? { path, cwd, lines: maxLines } : undefined);
}

interface GrepPreviewRow {
  loc: string;
  text: string;
}

function parseJsonResult(result: string | undefined): Record<string, unknown> | undefined {
  if (!result) return undefined;
  try {
    const value: unknown = JSON.parse(result);
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}

function inlineGrepRows(
  result: string | undefined,
): { rows: GrepPreviewRow[]; truncated: boolean } | undefined {
  const parsed = parseJsonResult(result);
  if (!parsed) return undefined;
  const rec = (value: unknown): Record<string, unknown> =>
    typeof value === "object" && value !== null ? (value as Record<string, unknown>) : {};
  const truncated = parsed.truncated === true;
  if (Array.isArray(parsed.matches)) {
    return {
      rows: parsed.matches.map((match) => ({
        loc: `${String(rec(match).path ?? "")}:${String(rec(match).line ?? "")}`,
        text: String(rec(match).text ?? ""),
      })),
      truncated,
    };
  }
  if (Array.isArray(parsed.files)) {
    return { rows: parsed.files.map((file) => ({ loc: String(file), text: "" })), truncated };
  }
  if (Array.isArray(parsed.counts)) {
    return {
      rows: parsed.counts.map((count) => ({
        loc: String(rec(count).path ?? ""),
        text: `${String(rec(count).count ?? 0)} matches`,
      })),
      truncated,
    };
  }
  return undefined;
}

export function useGrepToolPreview(tool: ToolCall, maxMatches: number) {
  const inline = inlineGrepRows(tool.result);
  const cwd = useActiveSessionCwd();
  const query =
    !inline && tool.name === "grep" && tool.fn && tool.fn !== "search" ? tool.fn : undefined;
  const { data } = useGrep(query ? { query, cwd, limit: maxMatches } : undefined);
  const rows =
    inline?.rows ??
    (data?.matches ?? []).map((match) => ({
      loc: `${match.path}:${match.lineNumber}`,
      text: match.text,
    }));
  const shown = rows.slice(0, maxMatches);
  return {
    shown,
    overflow: inline ? rows.length - shown.length : (data?.total ?? 0) - shown.length,
    truncated: inline?.truncated ?? false,
  };
}
