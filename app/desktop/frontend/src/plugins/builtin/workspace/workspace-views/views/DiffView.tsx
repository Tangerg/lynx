import type { Highlighter } from "shiki";
import type { WorkspaceDiffRow } from "@/plugins/builtin/workspace/application/workspaceData";
import { useMemo } from "react";
import { intraLineDiff } from "@/lib/diff/intraLineDiff";
import { stripCodeWrapper, useCodeHighlighter } from "@/lib/markdown/useCodeHighlight";
import { langFromPath, resolveLang } from "@/lib/markdown/shiki";
import { cn } from "@/lib/utils";

/** Unified (one column, +/− interleaved) vs split (old left, new right). */
export type DiffLayout = "unified" | "split";

// A diff row's line numbers plus a small ordinal are enough to produce a
// stable key without relying on row content collisions.
function keyFor(row: WorkspaceDiffRow, i: number): string {
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

// Word-level change tint — semi-transparent so the syntax foreground shows
// through. Applied to the exact changed sub-range of a replaced line (T2.2).
const WD_DEL_STYLE = "background-color:rgba(243,114,127,0.3);border-radius:2px";
const WD_ADD_STYLE = "background-color:rgba(30,215,96,0.3);border-radius:2px";

type WordDecoration = { start: number; end: number; properties: { style: string } };

// Highlight one line for inline injection into a grid row. `decorations` wrap
// the changed sub-range; Shiki splits the syntax token spans at the range
// bounds so the word tint composites over the syntax colours rather than
// replacing them.
function highlightInline(
  h: Highlighter,
  code: string,
  theme: string,
  lang: string,
  decorations: WordDecoration[],
): string {
  return stripCodeWrapper(h.codeToHtml(code || " ", { lang, theme, decorations }), code);
}

// Map each replaced line to its changed sub-range (word-level diff). Walks the
// flat rows pairing a run of deletes with the following run of adds
// row-for-row (the common edit shape); unpaired overflow lines get no mark.
function computeWordRanges(rows: WorkspaceDiffRow[]): Map<WorkspaceDiffRow, [number, number]> {
  const ranges = new Map<WorkspaceDiffRow, [number, number]>();
  let dels: Extract<WorkspaceDiffRow, { type: "deleted" }>[] = [];
  let adds: Extract<WorkspaceDiffRow, { type: "added" }>[] = [];
  const flush = () => {
    const n = Math.min(dels.length, adds.length);
    for (let i = 0; i < n; i++) {
      const { del, add } = intraLineDiff(dels[i]!.code, adds[i]!.code);
      if (del) ranges.set(dels[i]!, del);
      if (add) ranges.set(adds[i]!, add);
    }
    dels = [];
    adds = [];
  };
  for (const row of rows) {
    if (row.type === "deleted") dels.push(row);
    else if (row.type === "added") adds.push(row);
    else flush(); // hunk / context ends the run
  }
  flush();
  return ranges;
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

export function DiffView({
  rows,
  layout = "unified",
  path,
}: {
  rows: WorkspaceDiffRow[];
  layout?: DiffLayout;
  /** The diffed file's path — highlights each file in its OWN language
   *  (langFromPath) instead of assuming TypeScript. Omitted → "text". */
  path?: string;
}) {
  const { highlighter, theme: shikiTheme } = useCodeHighlighter();

  const wordRanges = useMemo(() => computeWordRanges(rows), [rows]);
  const highlighted = useMemo(() => {
    if (!highlighter) return null;
    const lang = resolveLang(highlighter, path ? langFromPath(path) : "text");
    const out = new Map<WorkspaceDiffRow, string>();
    for (const row of rows) {
      if (row.type === "hunk") continue;
      const range = wordRanges.get(row);
      const style =
        row.type === "deleted" ? WD_DEL_STYLE : row.type === "added" ? WD_ADD_STYLE : "";
      const decorations: WordDecoration[] =
        range && style ? [{ start: range[0], end: range[1], properties: { style } }] : [];
      out.set(row, highlightInline(highlighter, row.code, shikiTheme, lang, decorations));
    }
    return out;
  }, [highlighter, rows, shikiTheme, wordRanges, path]);

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
            <CodeCell code={row.code} html={highlighted?.get(row)} />
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
type Half = Extract<WorkspaceDiffRow, { type: "context" | "deleted" | "added" }> | null;
interface SplitRow {
  left: Half;
  right: Half;
}

// Fold the flat unified rows into aligned left/right pairs: a context line
// occupies both sides; a run of deletes pairs row-for-row with the following
// run of adds (the common edit shape), and the longer run leaves blank cells
// opposite its overflow. Hunk separators flush the pending runs and are
// rendered full-width by the caller.
function toSplitRows(rows: WorkspaceDiffRow[]): ({ hunk: string } | SplitRow)[] {
  const out: ({ hunk: string } | SplitRow)[] = [];
  let dels: Extract<WorkspaceDiffRow, { type: "deleted" }>[] = [];
  let adds: Extract<WorkspaceDiffRow, { type: "added" }>[] = [];
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
  rows: WorkspaceDiffRow[];
  highlighted: Map<WorkspaceDiffRow, string> | null;
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
  highlighted: Map<WorkspaceDiffRow, string> | null;
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
      <CodeCell code={row.code} html={highlighted?.get(row)} />
    </div>
  );
}
