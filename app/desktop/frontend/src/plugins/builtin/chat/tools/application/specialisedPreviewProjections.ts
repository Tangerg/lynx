import { parseJsonResult, resultLines } from "./toolResultParsing";

export interface SkillPreviewEntry {
  name: string;
  description: string;
}

export interface GlobPreviewModel {
  paths: string[];
}

export interface WebSearchPreviewResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

const SKILL_ENTRY = /<skill>\s*<name>([\s\S]*?)<\/name>\s*<description>([\s\S]*?)<\/description>/g;

export function projectSkillPreview(result: string | undefined): SkillPreviewEntry[] {
  return [...(result ?? "").matchAll(SKILL_ENTRY)].map((match) => ({
    name: match[1]!.trim(),
    description: match[2]!.trim(),
  }));
}

export function projectAskUserAnswer(result: string | undefined): string {
  const text = result?.trim();
  if (!text) return "";
  const parsed = parseJsonResult(result);
  if (!parsed) return text;
  const direct = parsed.answer ?? parsed.response;
  if (typeof direct === "string") return direct;
  const parts = Object.values(parsed).map((value) =>
    typeof value === "string"
      ? value
      : Array.isArray(value)
        ? value.filter((entry) => typeof entry === "string").join(", ")
        : "",
  );
  return parts.filter(Boolean).join(" · ") || text;
}

export function projectGlobPreview(result: string | undefined): GlobPreviewModel {
  const hits = parseJsonResult(result)?.hits;
  if (!Array.isArray(hits)) return { paths: [] };
  return { paths: hits.map(hitPath).filter((path) => path.length > 0) };
}

export function projectWebSearchPreview(result: string | undefined): WebSearchPreviewResult[] {
  const arr = parseJsonResult(result)?.results;
  if (!Array.isArray(arr)) return [];
  return arr.flatMap((entry) => {
    const result = record(entry);
    const url = typeof result.url === "string" ? result.url : "";
    if (!url) return [];
    return [
      {
        url,
        domain: domainOf(url),
        title: typeof result.title === "string" && result.title ? result.title : url,
        snippet: typeof result.snippet === "string" ? result.snippet : "",
      },
    ];
  });
}

function hitPath(hit: unknown): string {
  if (typeof hit === "string") return hit;
  return String(record(hit).path ?? "");
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : {};
}

function domainOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

// ── Text-returning tools ─────────────────────────────────────────────────────
//
// These four answer in prose the model reads, not JSON, so their projection is a
// parse of that prose. Each parser is anchored on the ONE piece of structure the
// runtime actually emits and degrades to "no structure found" otherwise, so a
// wording change on the backend costs a plain-text preview rather than a wrong one.

/** `search_memory`: `N. content`, one entry per recalled item, content may wrap. */
export function projectRecalledMemories(result: string | undefined): string[] {
  const entries: string[] = [];
  for (const line of resultLines(result)) {
    const start = /^\d+\.\s+(.*)$/.exec(line);
    if (start) entries.push(start[1]!);
    else if (entries.length > 0) entries[entries.length - 1] += `\n${line}`;
  }
  return entries;
}

export interface ConversationHit {
  speaker: string;
  day: string;
  snippet: string;
}

/** `search_conversations`: `N. [speaker · YYYY-MM-DD] snippet`. */
export function projectConversationHits(result: string | undefined): ConversationHit[] {
  const hits: ConversationHit[] = [];
  for (const line of resultLines(result)) {
    const parsed = /^\d+\.\s+\[([^·\]]+)·\s*([^\]]+)\]\s*(.*)$/.exec(line);
    if (parsed) {
      hits.push({ speaker: parsed[1]!.trim(), day: parsed[2]!.trim(), snippet: parsed[3]! });
    } else if (hits.length > 0) {
      hits[hits.length - 1]!.snippet += `\n${line}`;
    }
  }
  return hits;
}

export interface ToolSearchGroup {
  source: string;
  names: string[];
}

/** `search_tools`: prose, then `Not loaded:` and `  [source] a, b, c` per source. */
export function projectToolSearchGroups(result: string | undefined): ToolSearchGroup[] {
  const groups: ToolSearchGroup[] = [];
  for (const line of resultLines(result)) {
    const parsed = /^\s*\[([^\]]+)\]\s*(.+)$/.exec(line);
    if (!parsed) continue;
    const names = parsed[2]!
      .split(",")
      .map((name) => name.trim())
      .filter(Boolean);
    if (names.length > 0) groups.push({ source: parsed[1]!.trim(), names });
  }
  return groups;
}

// ── JSON-returning tools ─────────────────────────────────────────────────────

export interface SchedulePreview {
  id: string;
  title: string;
  cron: string;
  instructions: string;
  enabled: boolean;
  nextRunAt: string;
  lastRunAt: string;
}

/** `list_schedules` answers `{schedules: [...]}`, `create_schedule` `{schedule: {...}}`
 *  — one reader, because a preview showing one row and a preview showing many
 *  differ only in how many rows they got. */
export function projectSchedulePreviews(result: string | undefined): SchedulePreview[] {
  const parsed = parseJsonResult(result);
  const many = parsed?.schedules;
  const rows = Array.isArray(many) ? many : parsed?.schedule ? [parsed.schedule] : [];
  return rows.map((row) => {
    const s = record(row);
    return {
      id: text(s.schedule_id),
      title: text(s.title),
      cron: text(s.cron),
      instructions: text(s.instructions),
      enabled: s.enabled !== false,
      nextRunAt: text(s.next_run_at),
      lastRunAt: text(s.last_run_at),
    };
  });
}

/** `delete_schedule` answers the exact identity it removed. */
export function projectDeletedScheduleId(result: string | undefined): string {
  return text(parseJsonResult(result)?.schedule_id);
}

export interface HttpPreview {
  status: number;
  duration: string;
  truncated: boolean;
  headers: [string, string][];
  body: string;
}

/** `http_request` answers `{status, headers, body, truncated, duration}`. */
export function projectHttpPreview(result: string | undefined): HttpPreview | undefined {
  const parsed = parseJsonResult(result);
  if (typeof parsed?.status !== "number") return undefined;
  const headers = record(parsed.headers);
  return {
    status: parsed.status,
    duration: text(parsed.duration),
    truncated: parsed.truncated === true,
    headers: Object.entries(headers).map(([name, value]) => [name, text(value)]),
    body: text(parsed.body),
  };
}

export interface FetchedPage {
  content: string;
  format: string;
}

/** `web_fetch` answers `{content, format}` — markdown by default. */
export function projectFetchedPage(result: string | undefined): FetchedPage | undefined {
  const parsed = parseJsonResult(result);
  if (typeof parsed?.content !== "string") return undefined;
  return { content: parsed.content, format: text(parsed.format) || "text" };
}

export interface GoalToolPreview {
  objective: string;
  status: string;
  message: string;
}

/** `create_goal` and `get_goal` share the same model-facing goal view. */
export function projectGoalToolPreview(result: string | undefined): GoalToolPreview | undefined {
  const parsed = parseJsonResult(result);
  const goal = record(parsed?.goal);
  const objective = text(goal.objective);
  const message = text(parsed?.message);
  if (!objective && !message) return undefined;
  return {
    objective,
    status: text(goal.status),
    message,
  };
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}
