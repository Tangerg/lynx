// Bits shared by the built-in tool previews (index.tsx + specialised.tsx).

// Shared container shape for every inline tool preview. The wrapper lives
// inside a bg-surface card (the expanded activity row), so it uses no
// additional background — just padding and typography.
export const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-[12px] leading-[1.55] text-fg-muted";

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
