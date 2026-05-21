import type { DiffRow } from "@/components/tools/previews";
import { highlightTS } from "@/utils/highlight";

// A diff row's line numbers (`l`/`r`) plus a small ordinal are enough to
// produce a stable key without relying on row content collisions.
function keyFor(row: DiffRow, i: number): string {
  if (row.type === "hunk") return `h:${i}:${row.text}`;
  if (row.type === "add") return `+:${row.r}`;
  if (row.type === "del") return `-:${row.l}`;
  return `=:${row.l}-${row.r}`;
}

export function DiffView({ rows }: { rows: DiffRow[] }) {
  return (
    <div className="diff-view">
      {rows.map((row, i) => {
        const k = keyFor(row, i);
        if (row.type === "hunk") {
          return <div key={k} className="diff-hunk-head">{row.text}</div>;
        }
        const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
        const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
        const lnum = row.type === "del" ? row.l : row.r;
        return (
          <div key={k} className={`diff-line ${cls}`}>
            <span className="ln">{lnum}</span>
            <span className="sign">{sign}</span>
            <span className="code" dangerouslySetInnerHTML={{ __html: highlightTS(row.code) }} />
          </div>
        );
      })}
    </div>
  );
}
