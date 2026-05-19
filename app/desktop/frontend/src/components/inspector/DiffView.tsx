import type { DiffRow } from "@/components/tools/previews";
import { highlightTS } from "@/utils/highlight";

export function DiffView({ rows }: { rows: DiffRow[] }) {
  return (
    <div className="diff-view">
      {rows.map((row, i) => {
        if (row.type === "hunk") return <div key={i} className="diff-hunk-head">{row.text}</div>;
        const cls = row.type === "add" ? "add" : row.type === "del" ? "del" : "ctx";
        const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
        const lnum = row.type === "del" ? row.l : row.r;
        return (
          <div key={i} className={`diff-line ${cls}`}>
            <span className="ln">{lnum}</span>
            <span className="sign">{sign}</span>
            <span className="code" dangerouslySetInnerHTML={{ __html: highlightTS(row.code) }} />
          </div>
        );
      })}
    </div>
  );
}
