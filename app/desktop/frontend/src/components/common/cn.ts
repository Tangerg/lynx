// Minimal className combiner — no Tailwind merge magic, just join + dedupe falsy.
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter((x): x is string => Boolean(x)).join(" ");
}
