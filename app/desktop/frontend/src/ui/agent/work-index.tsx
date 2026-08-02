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
    <ScrollArea hideScrollbar className="px-[var(--density-navigation-gutter)] pb-5 pt-2">
      <div className="flex flex-col gap-y-[var(--density-navigation-section-gap)]">{children}</div>
    </ScrollArea>
  );
}

/** Isolates one plugin contribution so section spacing cannot leak into its children. */
export function AgentWorkIndexSection({ children }: { children: ReactNode }) {
  return <div className="min-w-0">{children}</div>;
}

/**
 * The drawer's anchor: what the agent is pointed at right now.
 *
 * Pinned above the scrolling index rather than contributed into it — the one
 * fact you must be able to read without scrolling is where a command is about to
 * run. Two lines, because a checkout's name without its location stops being an
 * answer the moment two of them share it.
 */
export function AgentWorkIndexIdentity({
  icon,
  name,
  detail,
}: {
  icon: ReactNode;
  name: ReactNode;
  detail?: ReactNode;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2.5 px-[var(--density-navigation-gutter)] pb-2 pt-1">
      <span className="grid size-5 shrink-0 place-items-center text-fg-muted">{icon}</span>
      <span className="flex min-w-0 flex-col gap-px">
        <span className="truncate text-ui-sm font-medium leading-snug text-fg">{name}</span>
        {detail != null && (
          <span className="truncate font-mono text-ui-2xs leading-snug text-fg-faint">
            {detail}
          </span>
        )}
      </span>
    </div>
  );
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
