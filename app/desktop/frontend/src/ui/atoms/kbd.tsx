import type { ReactNode } from "react";

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded-[4px] bg-surface-2 px-1 font-mono text-[10.5px] font-medium leading-none text-fg-faint shadow-[inset_0_0_0_0.5px_var(--color-field)]">
      {children}
    </kbd>
  );
}
