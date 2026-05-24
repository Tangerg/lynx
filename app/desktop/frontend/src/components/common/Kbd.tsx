import type { ReactNode } from "react";

// Keyboard hint glyph — used in search input + slash-suggestion hints.
export function Kbd({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex h-4.5 items-center rounded border border-line-soft bg-surface-2 px-1.5 font-mono text-[11px] text-fg-muted">
      {children}
    </span>
  );
}
