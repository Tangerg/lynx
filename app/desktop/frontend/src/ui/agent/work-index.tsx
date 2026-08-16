import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { ScrollArea } from "@/ui/atoms/scroll-area";

/**
 * Spatial patterns for the work index.
 *
 * Density tokens stay inside the agent UI layer so sidebar plugins compose
 * product content without knowing how a visual family spaces its navigation.
 */
export function AgentWorkIndexBody({ children }: { children: ReactNode }) {
  return (
    <ScrollArea
      hideScrollbar
      className="agent-index-scroll px-[var(--density-navigation-gutter)] pb-5 pt-2"
    >
      <div className="flex flex-col gap-y-[var(--density-navigation-section-gap)]">{children}</div>
    </ScrollArea>
  );
}

/** Isolates one plugin contribution so section spacing cannot leak into its children. */
export function AgentWorkIndexSection({ children }: { children: ReactNode }) {
  return <div className="min-w-0">{children}</div>;
}

export function AgentWorkIndexGroupList({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-[var(--density-navigation-group-gap)]", className)}>
      {children}
    </div>
  );
}

/** Pinned bottom of the drawer. No line above it: it is the same material as the
 *  index, and the whitespace plus the window edge already say where it ends. */
export function AgentWorkIndexFooter({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-1 bg-[var(--app-drawer-surface)] px-[var(--density-navigation-gutter)] pb-2.5 pt-2">
      {children}
    </div>
  );
}
