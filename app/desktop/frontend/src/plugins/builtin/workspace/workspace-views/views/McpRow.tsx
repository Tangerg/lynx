import type { MCPServer } from "@/lib/data/queries";
import type { IconName } from "@/components/common";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// MCP server row — appears in the Tools workspace view. Status pill
// (On / Idle / Error) tinted with semantic tokens; icon background
// mirrors the same tint for active / error states.
export function McpRow({ server }: { server: MCPServer }) {
  const label = server.status === "active" ? "On" : server.status === "idle" ? "Idle" : "Error";
  return (
    <div className="group grid grid-cols-[40px_1fr_auto_auto] items-center gap-3 px-4 py-3 hover:bg-surface">
      <div
        className={cn(
          "grid h-10 w-10 place-items-center rounded-lg bg-surface-2 text-fg-muted group-hover:bg-surface-3 group-hover:text-fg",
          server.status === "active" && "bg-accent/10 text-accent",
          server.status === "error" && "bg-negative/10 text-negative",
        )}
      >
        <Icon name={server.icon as IconName} size={15} />
      </div>
      <div className="min-w-0">
        <div className="text-[14px] font-semibold text-fg truncate">{server.name}</div>
        <div className="mt-0.5 text-[12px] text-fg-faint truncate">{server.desc}</div>
      </div>
      <div className="rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-fg-faint">
        {server.tools} tools
      </div>
      <div
        className={cn(
          "rounded-xs px-1.5 py-0.5 font-mono text-[11px] font-semibold",
          server.status === "active" && "bg-accent/12 text-accent",
          server.status === "idle" && "bg-surface-2 text-fg-faint",
          server.status === "error" && "bg-negative/12 text-negative",
        )}
      >
        {label}
      </div>
    </div>
  );
}
