import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function AgentStatusPill({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "running" | "warning" | "success";
}) {
  const dotClass =
    tone === "running"
      ? "bg-accent"
      : tone === "warning"
        ? "bg-warning"
        : tone === "success"
          ? "bg-success"
          : "bg-fg-faint";
  return (
    <span className="inline-flex h-[22px] items-center gap-1.5 rounded-full bg-surface px-2.5 font-sans text-[11px] font-medium leading-none text-fg-muted shadow-[inset_0_0_0_0.5px_var(--color-field)]">
      <span className={cn("h-1.5 w-1.5 rounded-full", dotClass)} />
      {children}
    </span>
  );
}
