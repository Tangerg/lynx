import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

type AgentPaneTone = "main" | "sidebar" | "rail" | "dock";

interface AgentPaneProps extends ComponentPropsWithoutRef<"div"> {
  tone?: AgentPaneTone;
}

export function AgentPane({ tone = "main", className, children, ...props }: AgentPaneProps) {
  return (
    <div {...props} className={cn("agent-pane", `agent-pane-${tone}`, className)}>
      {children}
    </div>
  );
}

export function AgentPaneHeader({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div {...props} className={cn("agent-pane-header", className)}>
      {children}
    </div>
  );
}
