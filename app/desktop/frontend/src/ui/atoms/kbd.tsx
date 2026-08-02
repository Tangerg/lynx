import type { ReactNode } from "react";

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="pointer-events-none inline-flex h-5 min-w-5 select-none items-center justify-center rounded-2xs bg-sunken px-1 font-sans text-ui-sm font-medium leading-none text-fg-muted">
      {children}
    </kbd>
  );
}
