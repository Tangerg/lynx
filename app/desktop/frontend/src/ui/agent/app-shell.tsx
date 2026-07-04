import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface AgentAppShellProps {
  rail?: boolean;
  sidebar?: ReactNode;
  main: ReactNode;
  overlay?: ReactNode;
  mode?: "work" | "single";
}

export function AgentAppShell({ rail, sidebar, main, overlay, mode = "work" }: AgentAppShellProps) {
  const single = mode === "single";
  return (
    <div
      className={cn("agent-app", rail && !single && "agent-app-rail", single && "agent-app-single")}
    >
      <div className="agent-shell-grid">
        {!single && (
          <aside aria-label="Work index" className="agent-region agent-region-sidebar">
            {sidebar}
          </aside>
        )}
        <main aria-label="Agent workspace" className="agent-region agent-region-main">
          {main}
        </main>
      </div>
      {overlay}
    </div>
  );
}
