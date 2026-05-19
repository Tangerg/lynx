import type { ReactNode } from "react";

// Keyboard hint glyph — used in search input and slash-suggestion hints.
export function Kbd({ children }: { children: ReactNode }) {
  return <span className="kbd">{children}</span>;
}
