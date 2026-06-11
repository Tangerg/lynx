import type { MCPServer } from "@/lib/data/queries";
import type { IconName } from "@/components/common";
import { Icon, IconButton } from "@/components/common";
import { getContainer } from "@/main/container";
import { cn } from "@/lib/utils";

// MCP server row — appears in the Tools workspace view. Status pill mirrors
// the wire lifecycle (AUX_API §5.1); the reconnect button's loading state is
// `status === "connecting"` — pushed via mcp.serverChanged, never invented
// locally (reconnect guarantees connecting → terminal ordering, §5.2).
const STATUS_PILL: Record<MCPServer["status"], { label: string; classes: string }> = {
  connecting: { label: "…", classes: "bg-surface-2 text-fg-muted animate-pulse" },
  connected: { label: "On", classes: "bg-accent/12 text-accent" },
  disconnected: { label: "Off", classes: "bg-surface-2 text-fg-faint" },
  failed: { label: "Error", classes: "bg-negative/12 text-negative" },
  needsAuth: { label: "Login", classes: "bg-warning/12 text-warning" },
};

function reconnect(server: string): void {
  // Fire-and-forget: progress arrives as mcp.serverChanged events → the
  // workspace-events plugin invalidates `mcp-servers` → this row re-renders
  // through the whole lifecycle without local state.
  getContainer()
    .client()
    .workspace.mcp.reconnect(server)
    .catch((err: unknown) => console.warn("[mcp] reconnect failed:", err));
}

export function McpRow({ server }: { server: MCPServer }) {
  const pill = STATUS_PILL[server.status];
  const connecting = server.status === "connecting";
  return (
    <div className="group grid grid-cols-[40px_1fr_auto_auto_auto] items-center gap-3 px-4 py-3 hover:bg-surface">
      <div
        className={cn(
          "grid h-10 w-10 place-items-center rounded-lg bg-surface-2 text-fg-muted group-hover:bg-surface-3 group-hover:text-fg",
          server.status === "connected" && "bg-accent/10 text-accent",
          server.status === "failed" && "bg-negative/10 text-negative",
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
        className={cn("rounded-xs px-1.5 py-0.5 font-mono text-[11px] font-semibold", pill.classes)}
        title={server.status === "failed" ? server.errorDetail : undefined}
      >
        {pill.label}
      </div>
      <IconButton
        title="Reconnect"
        disabled={connecting}
        onClick={() => reconnect(server.id)}
        className={cn(connecting && "animate-spin")}
      >
        <Icon name="loop" size={13} />
      </IconButton>
    </div>
  );
}
