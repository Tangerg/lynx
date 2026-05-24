import type { DiffRow } from "@/components/tools/previews";
import { cn } from "@/lib/utils";
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
          row.type === "add" ? "bg-[rgba(30,215,96,0.07)]" :
          row.type === "del" ? "bg-[rgba(243,114,127,0.07)]" :
          "";
        const meta =
          row.type === "add" ? "text-[rgba(95,227,154,0.7)]" :
          row.type === "del" ? "text-[rgba(243,114,127,0.7)]" :
          "text-fg-faint";
        const codeTone =
          row.type === "add" ? "text-[#c8f5d8]" :
          row.type === "del" ? "text-[#f5cdd2]" :
          "text-fg-soft";
        const sign = row.type === "add" ? "+" : row.type === "del" ? "−" : " ";
        const lnum = row.type === "del" ? row.l : row.r;
        return (
          <div
            key={k}
            className={cn("grid grid-cols-[36px_36px_1fr] gap-1.5 px-3", tone)}
          >
            <span className={cn("text-right text-[11px] select-none", meta)}>{lnum}</span>
            <span className={cn("text-center text-[11px] select-none", meta)}>{sign}</span>
            <span
              className={cn("whitespace-pre", codeTone)}
              dangerouslySetInnerHTML={{ __html: highlightTS(row.code) }}
            />
          </div>
        );
      })}
    </div>
  );
}
