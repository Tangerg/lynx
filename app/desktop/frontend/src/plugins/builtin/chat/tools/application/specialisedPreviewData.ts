import { parseJsonResult } from "./toolResultParsing";

export interface SkillPreviewEntry {
  name: string;
  description: string;
}

export interface GlobPreviewData {
  paths: string[];
  truncated: boolean;
}

export interface WebSearchPreviewResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

const SKILL_ENTRY = /<skill>\s*<name>([\s\S]*?)<\/name>\s*<description>([\s\S]*?)<\/description>/g;

export function skillPreviewEntries(result: string | undefined): SkillPreviewEntry[] {
  return [...(result ?? "").matchAll(SKILL_ENTRY)].map((match) => ({
    name: match[1]!.trim(),
    description: match[2]!.trim(),
  }));
}

export function askUserPreviewAnswer(result: string | undefined): string {
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

export function globPreviewData(result: string | undefined): GlobPreviewData {
  const parsed = parseJsonResult(result);
  const arr = [parsed?.hits, parsed?.matches, parsed?.files, parsed?.paths].find(Array.isArray);
  if (!arr) return { paths: [], truncated: parsed?.truncated === true };
  return {
    paths: (arr as unknown[]).map(hitPath).filter((path) => path.length > 0),
    truncated: parsed?.truncated === true,
  };
}

export function lspPreviewOperation(args: string | undefined): string {
  const parsed = parseJsonResult(args);
  return typeof parsed?.operation === "string" ? parsed.operation : "";
}

export function webSearchPreviewResults(result: string | undefined): WebSearchPreviewResult[] {
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
