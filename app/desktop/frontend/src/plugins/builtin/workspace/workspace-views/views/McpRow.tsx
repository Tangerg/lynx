import type { MCPServer } from "@/lib/data/queries";
import type { IconName } from "@/components/common";
import { useState } from "react";
import { Icon, IconButton } from "@/components/common";
import { useMCPTools } from "@/lib/data/queries";
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

// Hand the backend a bearer token for a needsAuth server (B12, docs/613).
// Fire-and-forget like reconnect: the backend reconnects with the token and
// pushes connecting → (connected | needsAuth | failed) via mcp.serverChanged,
// so the row re-renders through the lifecycle with no local state.
function authenticate(server: string, token: string): void {
  getContainer()
    .client()
    .workspace.mcp.authenticate({ server, token })
    .catch((err: unknown) => console.warn("[mcp] authenticate failed:", err));
}

// Expanded detail: the server's tool list (workspace.mcp.listTools), fetched
// lazily on first expand and kept fresh by mcp.serverChanged invalidation.
function McpToolList({ server }: { server: string }) {
  const { data: tools, isLoading } = useMCPTools({ server });
  if (isLoading)
    return <p className="m-0 px-4 pb-3 pl-[68px] text-[11.5px] text-fg-faint">Loading tools…</p>;
  if (!tools?.length)
    return <p className="m-0 px-4 pb-3 pl-[68px] text-[11.5px] text-fg-faint">No tools exposed.</p>;
  return (
    <ul className="m-0 list-none px-4 pb-3 pl-[68px]">
      {tools.map((tool) => (
        <li key={tool.name} className="flex items-baseline gap-2 py-0.5">
          <code className="shrink-0 rounded-xs bg-surface-2 px-1 font-mono text-[11px] text-fg">
            {tool.name}
          </code>
          <span className="truncate text-[11.5px] text-fg-faint">{tool.description}</span>
        </li>
      ))}
    </ul>
  );
}

// Token entry for a needsAuth server — the front half of the OAuth dance (the
// browser flow, if any) is the user's; lyra only forwards the resulting token.
function McpAuthForm({ server }: { server: string }) {
  const [token, setToken] = useState("");
  const submit = () => {
    if (!token.trim()) return;
    authenticate(server, token.trim());
    setToken("");
  };
  return (
    <div className="flex items-center gap-2 px-4 pb-3 pl-[68px]">
      <input
        type="password"
        aria-label={`${server} access token`}
        value={token}
        onChange={(e) => setToken(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && submit()}
        placeholder="Paste access token…"
        className="h-8 min-w-0 flex-1 rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent"
      />
      <button
        type="button"
        disabled={!token.trim()}
        onClick={submit}
        className={cn(
          "h-8 shrink-0 rounded-md px-3 text-[12px] font-semibold transition-colors",
          token.trim()
            ? "bg-accent text-on-accent hover:opacity-90"
            : "cursor-not-allowed bg-surface-2 text-fg-faint",
        )}
      >
        Authenticate
      </button>
    </div>
  );
}

export function McpRow({ server }: { server: MCPServer }) {
  const pill = STATUS_PILL[server.status];
  const connecting = server.status === "connecting";
  // Click the row to expand its tool list — the "N tools" badge finally has
  // a detail behind it.
  const [open, setOpen] = useState(false);
  return (
    <div>
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
        {/* The name/desc block is the expand toggle (a nested button inside a
            row-button would be invalid HTML — IconButton sits beside it). */}
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="min-w-0 border-0 bg-transparent p-0 text-left"
        >
          <div className="text-[14px] font-semibold text-fg truncate">{server.name}</div>
          <div className="mt-0.5 text-[12px] text-fg-faint truncate">{server.desc}</div>
        </button>
        <div className="rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-fg-faint">
          {server.tools} tools
        </div>
        <div
          className={cn(
            "rounded-xs px-1.5 py-0.5 font-mono text-[11px] font-semibold",
            pill.classes,
          )}
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
      {open &&
        (server.status === "needsAuth" ? (
          <McpAuthForm server={server.id} />
        ) : (
          <McpToolList server={server.id} />
        ))}
    </div>
  );
}
