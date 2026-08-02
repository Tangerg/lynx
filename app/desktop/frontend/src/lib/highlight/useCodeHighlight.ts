// Shared Shiki plumbing for the code renderers (markdown ShikiCodeBlock, the
// diff view, the file viewer). Each renders differently — whole-block with a
// cache, per-row with decorations, whole-file split — but the theme resolution
// and (for the two view renderers) the load-into-state effect are identical, so
// they live here instead of being re-spelled in each.

import type { Highlighter } from "shiki";
import { useEffect, useState } from "react";
import { useScheme } from "../appearance";
import { getHighlighter } from "./shiki";

/** The Shiki theme preset matching the active UI scheme. Keyed on the scheme,
 *  never the theme id, so third-party light themes ("solarized-light" etc.)
 *  also pick the right preset. */
export function useShikiTheme(): string {
  return useScheme() === "light" ? "github-light-high-contrast" : "github-dark";
}

/** The shared highlighter loaded into state (null until ready) plus the active
 *  theme — the common setup for the DiffView / FileView renderers. (Markdown's
 *  ShikiCodeBlock has its own cache-first load and uses [useShikiTheme] only.) */
export function useCodeHighlighter(): { highlighter: Highlighter | null; theme: string } {
  const theme = useShikiTheme();
  const [highlighter, setHighlighter] = useState<Highlighter | null>(null);
  useEffect(() => {
    let cancelled = false;
    void getHighlighter().then((h) => {
      if (!cancelled) setHighlighter(h);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return { highlighter, theme };
}

/** Extract the inner HTML of Shiki's <pre><code>…</code></pre> so the token
 *  spans can be injected into a custom grid row; returns `fallback` on no
 *  match. */
export function stripCodeWrapper(html: string, fallback: string): string {
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? fallback;
}
