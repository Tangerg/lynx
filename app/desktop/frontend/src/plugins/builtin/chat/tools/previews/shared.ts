// Bits shared by the built-in tool previews (index.tsx + specialised.tsx).

// Shared container shape for every inline tool preview. The wrapper sits
// one step deeper than the .tool-card surface (canvas) so it reads as
// nested content — DESIGN.md §5 surface-step depth rule.
export const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto bg-canvas px-3.5 pt-2.5 pb-2 font-mono text-[12px] leading-[1.55] text-fg-muted";

/** Non-empty trimmed lines of a tool's text result ([] while absent). */
export function resultLines(result: string | undefined): string[] {
  const t = result?.trim();
  return t ? t.split("\n") : [];
}

/** Best-effort parse of a JSON-object tool result; undefined on anything
 *  else (partial streaming output, plain-text results, arrays). */
export function parseJsonResult(result: string | undefined): Record<string, unknown> | undefined {
  if (!result) return undefined;
  try {
    const v: unknown = JSON.parse(result);
    return typeof v === "object" && v !== null && !Array.isArray(v)
      ? (v as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}
