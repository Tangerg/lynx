import type { Highlighter } from "shiki";
import type { DiffRow } from "@/lib/data/queries";
import { useEffect, useMemo, useState } from "react";
import { getHighlighter } from "@/lib/markdown/shiki";
import { cn } from "@/lib/utils";
import { resolveScheme } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

/** Unified (one column, +/− interleaved) vs split (old left, new right). */
export type DiffLayout = "unified" | "split";

// A diff row's line numbers plus a small ordinal are enough to produce a
// stable key without relying on row content collisions.
function keyFor(row: DiffRow, i: number): string {
  if (row.type === "hunk") return `h:${i}:${row.text}`;
  if (row.type === "added") return `+:${row.rightLine}`;
  if (row.type === "deleted") return `-:${row.leftLine}`;
  return `=:${row.leftLine}-${row.rightLine}`;
}

// Per-line presentation keyed by row type — one lookup beats three parallel
// ternary chains switching on the same field.
const ROW_STYLE: Record<
  "added" | "deleted" | "context",
  { tone: string; meta: string; sign: string }
> = {
  added: { tone: "bg-[rgba(30,215,96,0.07)]", meta: "text-[rgba(95,227,154,0.7)]", sign: "+" },
  deleted: { tone: "bg-[rgba(243,114,127,0.07)]", meta: "text-[rgba(243,114,127,0.7)]", sign: "−" },
  context: { tone: "", meta: "text-fg-faint", sign: " " },
};

// Strip Shiki's <pre><code>…</code></pre> wrapper so the inner token spans
// can be injected inline into our grid row.
function highlightInline(h: Highlighter, code: string, theme: string): string {
  const html = h.codeToHtml(code || " ", { lang: "typescript", theme });
  return html.match(/<code[^>]*>([\s\S]*)<\/code>/)?.[1] ?? code;
}

// The code cell — Shiki-highlighted token spans when ready, else the plain
// text. `whitespace-pre` keeps indentation; the parent grid column is
// `minmax(0,1fr)` so a long line scrolls (unified) / clips (split) instead of
// blowing the column out.
function CodeCell({ code, html }: { code: string; html: string | undefined }) {
  return html ? (
    <span className="overflow-hidden whitespace-pre" dangerouslySetInnerHTML={{ __html: html }} />
  ) : (
    <span className="overflow-hidden whitespace-pre text-fg-soft">{code}</span>
  );
}

export function DiffView({ rows, layout = "unified" }: { rows: DiffRow[]; layout?: DiffLayout }) {
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

  if (layout === "split") {
    return <SplitDiff rows={rows} highlighted={highlighted} />;
  }

  return (
    <div className="py-2 font-mono text-[12px] leading-[1.6]">
      {rows.map((row, i) => {
        const k = keyFor(row, i);
        if (row.type === "hunk") return <HunkRow key={k} text={row.text} />;
        const style = ROW_STYLE[row.type];
        const lnum = row.type === "deleted" ? row.leftLine : row.rightLine;
        return (
          <div key={k} className={cn("grid grid-cols-[36px_36px_1fr] gap-1.5 px-3", style.tone)}>
            <span className={cn("text-right text-[11px] select-none", style.meta)}>{lnum}</span>
            <span className={cn("text-center text-[11px] select-none", style.meta)}>
              {style.sign}
            </span>
            <CodeCell code={row.code} html={highlighted?.get(row.code)} />
          </div>
        );
      })}
    </div>
  );
}

function HunkRow({ text }: { text: string }) {
  return (
    <div className="mx-0 mt-2.5 mb-0 border-0 bg-line px-3 py-1 text-[11px] text-fg-faint">
      {text}
    </div>
  );
}

// ── split (side-by-side) ────────────────────────────────────────────────────

// One side of a split row: a context/deleted cell on the left, a
// context/added cell on the right, or absent (the other side changed and this
// side has no counterpart).
type Half = Extract<DiffRow, { type: "context" | "deleted" | "added" }> | null;
interface SplitRow {
  left: Half;
  right: Half;
}

// Fold the flat unified rows into aligned left/right pairs: a context line
// occupies both sides; a run of deletes pairs row-for-row with the following
// run of adds (the common edit shape), and the longer run leaves blank cells
// opposite its overflow. Hunk separators flush the pending runs and are
// rendered full-width by the caller.
function toSplitRows(rows: DiffRow[]): ({ hunk: string } | SplitRow)[] {
  const out: ({ hunk: string } | SplitRow)[] = [];
  let dels: Extract<DiffRow, { type: "deleted" }>[] = [];
  let adds: Extract<DiffRow, { type: "added" }>[] = [];
  const flush = () => {
    const n = Math.max(dels.length, adds.length);
    for (let i = 0; i < n; i++) out.push({ left: dels[i] ?? null, right: adds[i] ?? null });
    dels = [];
    adds = [];
  };
  for (const row of rows) {
    if (row.type === "hunk") {
      flush();
      out.push({ hunk: row.text });
    } else if (row.type === "context") {
      flush();
      out.push({ left: row, right: row });
    } else if (row.type === "deleted") {
      dels.push(row);
    } else {
      adds.push(row);
    }
  }
  flush();
  return out;
}

function SplitDiff({
  rows,
  highlighted,
}: {
  rows: DiffRow[];
  highlighted: Map<string, string> | null;
}) {
  const split = useMemo(() => toSplitRows(rows), [rows]);
  return (
    <div className="py-2 font-mono text-[12px] leading-[1.6]">
      {split.map((row, i) => {
        if ("hunk" in row) return <HunkRow key={`h:${i}`} text={row.hunk} />;
        return (
          <div key={`s:${i}`} className="grid grid-cols-2">
            <DiffSide row={row.left} side="left" highlighted={highlighted} />
            <DiffSide row={row.right} side="right" highlighted={highlighted} />
          </div>
        );
      })}
    </div>
  );
}

function DiffSide({
  row,
  side,
  highlighted,
}: {
  row: Half;
  side: "left" | "right";
  highlighted: Map<string, string> | null;
}) {
  // Absent counterpart — a faint "no line here" fill so the eye reads the row
  // as one-sided rather than as an edit to a blank line.
  if (!row) return <div className="bg-[rgba(128,128,128,0.045)]" />;
  const style = ROW_STYLE[row.type];
  // deleted lives on the left, added on the right; a context line shows this
  // side's own number. A context row keeps no +/− sign (it's unchanged).
  const lnum =
    row.type === "deleted"
      ? row.leftLine
      : row.type === "added"
        ? row.rightLine
        : side === "left"
          ? row.leftLine
          : row.rightLine;
  const sign = row.type === "context" ? "" : style.sign;
  return (
    <div className={cn("grid grid-cols-[34px_16px_minmax(0,1fr)] gap-1.5 px-3", style.tone)}>
      <span className={cn("text-right text-[11px] select-none", style.meta)}>{lnum}</span>
      <span className={cn("text-center text-[11px] select-none", style.meta)}>{sign}</span>
      <CodeCell code={row.code} html={highlighted?.get(row.code)} />
    </div>
  );
}
