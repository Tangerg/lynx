import type { ReactNode } from "react";

// Keyboard hint glyph — used in search input + slash-suggestion hints.
export function Kbd({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex h-[18px] items-center rounded border border-line-soft bg-surface-2 px-[6px] font-mono text-[10.5px] text-fg-muted">
      {children}
    </span>
  );
}
