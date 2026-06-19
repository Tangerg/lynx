import type { Highlighter } from "shiki";
import { useEffect, useMemo, useRef, useState } from "react";
import { getHighlighter } from "@/lib/markdown/shiki";
import { cn } from "@/lib/utils";
import { resolveScheme } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

// Whole-file viewer (workspace.readFile) — the target of a clickable file:line
// reference. The file is highlighted in ONE Shiki pass and split into per-line
// HTML (Shiki separates source lines by a literal newline inside <code>), so a
// large file costs one highlight call, not one per line. The target line is
// scrolled to centre and tinted. Lang is hardcoded like DiffView — approximate
// highlighting on non-TS files, but text/line-numbers/scroll are exact.

// Split a full highlight into per-line inner HTML by stripping the <pre><code>
// wrapper and splitting on the newlines Shiki places between line spans.
function highlightLines(h: Highlighter, code: string, theme: string): string[] {
  const html = h.codeToHtml(code, { lang: "typescript", theme });
  const inner = html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? "";
  return inner.split("\n");
}

export function FileView({ content, targetLine }: { content: string; targetLine: number }) {
  const themeId = useUiStore((s) => s.theme);
  const shikiTheme = resolveScheme(themeId) === "light" ? "github-light" : "github-dark";
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

  // Plain lines drive the gutter + the fallback render; the highlighted variant
  // (when ready) renders inline. Both have the same length, so they align.
  const plain = useMemo(() => content.split("\n"), [content]);
  const highlighted = useMemo(
    () => (highlighter ? highlightLines(highlighter, content, shikiTheme) : null),
    [highlighter, content, shikiTheme],
  );

  // Centre the target line once it (and the content) are in the DOM.
  const targetRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (targetLine > 0) targetRef.current?.scrollIntoView({ block: "center" });
  }, [targetLine, highlighted]);

  return (
    <div className="py-2 font-mono text-[12px] leading-[1.6]">
      {plain.map((line, i) => {
        const n = i + 1;
        const isTarget = n === targetLine;
        const html = highlighted?.[i];
        return (
          <div
            key={i}
            ref={isTarget ? targetRef : undefined}
            className={cn(
              "grid grid-cols-[44px_1fr] gap-2 px-3",
              isTarget && "bg-[color-mix(in_srgb,var(--color-accent)_12%,transparent)]",
            )}
          >
            <span className="text-right text-[11px] text-fg-faint select-none">{n}</span>
            {html !== undefined ? (
              <span
                className="overflow-hidden whitespace-pre"
                dangerouslySetInnerHTML={{ __html: html }}
              />
            ) : (
              <span className="overflow-hidden whitespace-pre text-fg-soft">{line || " "}</span>
            )}
          </div>
        );
      })}
    </div>
  );
}
