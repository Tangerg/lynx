import type { IconName } from "@/ui";
import { useState } from "react";
import { Icon, IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { cn } from "@/lib/utils";
import {
  reconnectMCPServer,
  type MCPServerConfig,
  useMCPServerToolConfigs,
} from "@/plugins/builtin/workspace/application/toolCatalog";

// MCP server row — appears in the Tools workspace view. Status pill mirrors
// the wire lifecycle (AUX_API §5.1); the reconnect button's loading state is
// `status === "connecting"` — pushed via mcp.serverChanged, never invented
// locally (reconnect guarantees connecting → terminal ordering, §5.2).
// i18n key → pill classes. Labels are resolved at render via t().
const STATUS_CLASSES: Record<MCPServerConfig["status"], { key: string; classes: string }> = {
  connecting: {
    key: "tools.status.connecting",
    classes: "bg-surface-2 text-fg-muted animate-pulse",
  },
  connected: { key: "tools.status.on", classes: "bg-accent/12 text-accent" },
  disconnected: { key: "tools.status.off", classes: "bg-surface-2 text-fg-faint" },
  failed: { key: "tools.status.error", classes: "bg-negative/12 text-negative" },
  needsAuth: { key: "tools.status.login", classes: "bg-warning/12 text-warning" },
};

// Expanded detail: the server's tool list (mcp.tools.list), fetched
// lazily on first expand and kept fresh by mcp.serverChanged invalidation.
function McpToolList({ server }: { server: string }) {
  const t = useT();
  const { data: tools, isLoading } = useMCPServerToolConfigs(server);
  if (isLoading)
    return (
      <p className="m-0 px-4 pb-3 pl-[68px] text-[11.5px] text-fg-faint">
        {t("tools.loadingTools")}
      </p>
    );
  if (!tools?.length)
    return (
      <p className="m-0 px-4 pb-3 pl-[68px] text-[11.5px] text-fg-faint">{t("tools.noTools")}</p>
    );
  return (
    <ul className="m-0 list-none px-4 pb-3 pl-[68px]">
      {tools.map((tool) => (
        <li key={tool.name} className="flex items-baseline gap-2 py-0.5">
          <code className="shrink-0 rounded-sm bg-surface-2 px-1 font-mono text-[11px] text-fg">
            {tool.name}
          </code>
          <span className="truncate text-[11.5px] text-fg-faint" title={tool.description}>
            {tool.description}
          </span>
        </li>
      ))}
    </ul>
  );
}

// A needsAuth server needs a bearer token, which is part of its persisted
// config now (set as `authorization` via mcp.configs.configure) — not a
// separate one-shot handoff. So this row just routes the user to the MCP
// settings pane, deep-linked, rather than holding its own token field.
function McpAuthGuide({ server }: { server: string }) {
  const t = useT();
  const openConfig = () => {
    openWorkspaceSettingsPane("mcp-servers");
  };
  return (
    <div className="flex items-center gap-2 px-4 pb-3 pl-[68px]">
      <button
        type="button"
        onClick={openConfig}
        className="inline-flex items-center gap-1.5 text-[12px] text-fg-muted hover:text-fg"
      >
        <Icon name="settings" size={13} />
        {t("tools.auth.configure", { server })}
      </button>
    </div>
  );
}

export function McpRow({ server }: { server: MCPServerConfig }) {
  const t = useT();
  const pill = STATUS_CLASSES[server.status];
  const connecting = server.status === "connecting";
  // Click the row to expand its tool list — the "N tools" badge finally has
  // a detail behind it.
  const [open, setOpen] = useState(false);
  return (
    <div>
      <div className="group grid grid-cols-[40px_1fr_auto_auto_auto] items-center gap-3 px-4 py-3 hover:bg-fg/[0.04]">
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
        <div className="rounded-sm bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-fg-faint">
          {server.tools} tools
        </div>
        <div
          className={cn(
            "rounded-sm px-1.5 py-0.5 font-mono text-[11px] font-semibold",
            pill.classes,
          )}
          title={server.status === "failed" ? server.errorDetail : undefined}
        >
          {t(pill.key)}
        </div>
        <IconButton
          title={t("tools.reconnect")}
          disabled={connecting}
          onClick={() => reconnectMCPServer(server.id)}
          className={cn(connecting && "animate-spin")}
        >
          <Icon name="loop" size={13} />
        </IconButton>
      </div>
      {open &&
        (server.status === "needsAuth" ? (
          <McpAuthGuide server={server.id} />
        ) : (
          <McpToolList server={server.id} />
        ))}
    </div>
  );
}
