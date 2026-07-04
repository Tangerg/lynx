// Shared list primitives for the Diagnostics panels — the virtualized list and
// its row/cell/empty chrome. Traces + Logs both render through VirtualList;
// Metrics draws its own table but shares Empty. Kept in one place so the two
// list tabs can't drift on row rhythm or the measure-element wiring.

import type { ReactNode } from "react";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";

export function Empty({ hint }: { hint: string }) {
  return <div className="text-[13px] text-fg-faint">No data yet — {hint}</div>;
}

export function Row({
  children,
  head,
  className,
}: {
  children: ReactNode;
  head?: boolean;
  className?: string;
}) {
  return (
    <div
      className={
        "flex items-center gap-3 px-1 font-mono text-[12px] " +
        (head ? "text-[10px] text-fg-faint" : "text-fg hover:bg-fg/[0.04]") +
        (className ? " " + className : "")
      }
    >
      {children}
    </div>
  );
}

export function Cell({ className, children }: { className: string; children?: ReactNode }) {
  return <div className={`min-w-0 ${className}`}>{children}</div>;
}

// Virtualized list — only on-screen rows mount, so a 500-row span buffer
// renders a dozen DOM nodes. `rowHeight` is just the pre-measure estimate;
// each row's real height is measured via `measureElement` so a row that
// expands (the span detail panel) grows in place and pushes the rest down,
// no fixed-height assumption. `position: absolute` per row is the standard
// react-virtual pattern (allowed by the no-absolute rule for this).
export function VirtualList({
  count,
  rowHeight,
  header,
  renderRow,
}: {
  count: number;
  rowHeight: number;
  header: ReactNode;
  renderRow: (index: number) => ReactNode;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      {header}
      <div ref={parentRef} className="flex-1 min-h-0 overflow-y-auto">
        <div className="relative w-full" style={{ height: virt.getTotalSize() }}>
          {virt.getVirtualItems().map((vi) => (
            <div
              key={vi.key}
              data-index={vi.index}
              ref={virt.measureElement}
              className="absolute left-0 top-0 w-full"
              style={{ transform: `translateY(${vi.start}px)` }}
            >
              {renderRow(vi.index)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
