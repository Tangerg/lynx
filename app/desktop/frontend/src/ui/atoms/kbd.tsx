import type { ReactNode } from "react";

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded-[5px] bg-surface-2 px-1.5 font-mono text-[11px] font-medium leading-none text-fg-muted">
      {children}
    </kbd>
  );
}
