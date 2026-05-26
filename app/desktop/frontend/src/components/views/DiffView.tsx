import type { Highlighter } from "shiki";
import type { DiffRow } from "@/lib/queries";
import { useEffect, useMemo, useState } from "react";
import { getHighlighter } from "@/lib/shiki";
import { cn } from "@/lib/utils";
import { resolveScheme } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

// A diff row's line numbers (`l`/`r`) plus a small ordinal are enough to
// produce a stable key without relying on row content collisions.
function keyFor(row: DiffRow, i: number): string {
  if (row.type === "hunk") return `h:${i}:${row.text}`;
  if (row.type === "add") return `+:${row.r}`;
  if (row.type === "del") return `-:${row.l}`;
  return `=:${row.l}-${row.r}`;
}

// Strip Shiki's <pre><code>…</code></pre> wrapper so the inner token spans
// can be injected inline into our grid row.
function highlightInline(h: Highlighter, code: string, theme: string): string {
  const html = h.codeToHtml(code || " ", { lang: "typescript", theme });
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? code;
}

export function DiffView({ rows }: { rows: DiffRow[] }) {
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

  const highlighted = useMemo(() => {
    if (!highlighter) return null;
    const out = new Map<string, string>();
    for (const row of rows) {
      if (row.type !== "hunk")
        out.set(row.code, highlightInline(highlighter, row.code, shikiTheme));
    }
    return out;
  }, [highlighter, rows, shikiTheme]);

  return (
    <div className="py-2 font-mono text-[12px] leading-[1.6]">
      {rows.map((row, i) => {
        const k = keyFor(row, i);
        if (row.type === "hunk") {
          return (
            <div
              key={k}
              className="mx-0 mt-2.5 mb-0 border-0 bg-line px-3 py-1 text-[11px] text-fg-faint"
            >
              {row.text}
            </div>
          );
        }
        const tone =
          row.type === "add"
            ? "bg-[rgba(30,215,96,0.07)]"
            : row.type === "del"
              ? "bg-[rgba(243,114,127,0.07)]"
              : "";
        const meta =
          row.type === "add"
            ? "text-[rgba(95,227,154,0.7)]"
            : row.type === "del"
              ? "text-[rgba(243,114,127,0.7)]"
              : "text-fg-faint";
        const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
        const lnum = row.type === "del" ? row.l : row.r;
        const inner = highlighted?.get(row.code);
        return (
          <div key={k} className={cn("grid grid-cols-[36px_36px_1fr] gap-1.5 px-3", tone)}>
            <span className={cn("text-right text-[11px] select-none", meta)}>{lnum}</span>
            <span className={cn("text-center text-[11px] select-none", meta)}>{sign}</span>
            {inner ? (
              <span className="whitespace-pre" dangerouslySetInnerHTML={{ __html: inner }} />
            ) : (
              <span className="whitespace-pre text-fg-soft">{row.code}</span>
            )}
          </div>
        );
      })}
    </div>
  );
}
