import { MCP_SERVERS_PANE } from "@/plugins/builtin/settings/public/panes";
import type { IconName } from "@/ui";
import { useId, useRef, useState } from "react";
import { Icon, IconButton, Pressable, TextButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { cn } from "@/lib/classNames";
import {
  type MCPServerSettings,
  mcpServerMutationWasRetired,
  reconnectMCPServer,
} from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { useMCPServerToolConfigs } from "@/plugins/builtin/workspace/application/toolCatalog";

// MCP server row — appears in the Tools workspace view. Status pill mirrors
// the wire lifecycle (AUX_API §5.1). The button adds only an admission latch
// until the owner repairs that projection; connecting → terminal remains the
// Runtime event stream's authoritative state (§5.2).
// i18n key → pill classes. Labels are resolved at render via t().
const STATUS_CLASSES: Record<MCPServerSettings["status"], { key: string; classes: string }> = {
  disabled: { key: "tools.status.off", classes: "bg-surface-2 text-fg-faint" },
  connecting: {
    key: "tools.status.connecting",
    classes: "bg-surface-2 text-fg-muted animate-pulse",
  },
  connected: { key: "tools.status.on", classes: "bg-accent-wash text-accent" },
  disconnected: { key: "tools.status.off", classes: "bg-surface-2 text-fg-faint" },
  failed: { key: "tools.status.error", classes: "bg-negative-wash text-negative" },
  needsAuth: { key: "tools.status.login", classes: "bg-warning-wash text-warning" },
};

// Expanded detail: the server's tool list (mcp.tools.list), fetched
// lazily on first expand and kept fresh by mcp.serverChanged invalidation.
function McpToolList({ server }: { server: string }) {
  const t = useT();
  const { data: tools, isLoading } = useMCPServerToolConfigs(server);
  if (isLoading)
    return (
      <p className="m-0 px-4 pb-3 pl-[68px] text-ui-sm text-fg-faint">{t("tools.loadingTools")}</p>
    );
  if (!tools?.length)
    return <p className="m-0 px-4 pb-3 pl-[68px] text-ui-sm text-fg-faint">{t("tools.noTools")}</p>;
  return (
    <ul className="m-0 list-none px-4 pb-3 pl-[68px]">
      {tools.map((tool) => (
        <li key={tool.name} className="flex items-baseline gap-2 py-0.5">
          <code className="shrink-0 rounded-sm bg-surface-2 px-1 font-mono text-ui-sm text-fg">
            {tool.name}
          </code>
          <span className="truncate text-ui-sm text-fg-faint" title={tool.description}>
            {tool.description}
          </span>
        </li>
      ))}
    </ul>
  );
}

// A needsAuth server needs a bearer token, which is part of its persisted
// connection config — not a
// separate one-shot handoff. So this row just routes the user to the MCP
// settings pane, deep-linked, rather than holding its own token field.
function McpAuthGuide({ server }: { server: string }) {
  const t = useT();
  const openConfig = () => {
    openWorkspaceSettingsPane(MCP_SERVERS_PANE);
  };
  return (
    <div className="flex items-center gap-2 px-4 pb-3 pl-[68px]">
      <TextButton onClick={openConfig}>
        <Icon name="settings" size="sm" />
        {t("tools.auth.configure", { server })}
      </TextButton>
    </div>
  );
}

export function McpRow({ server }: { server: MCPServerSettings }) {
  const t = useT();
  const pill = STATUS_CLASSES[server.status];
  const reconnectingRef = useRef(false);
  const [reconnecting, setReconnecting] = useState(false);
  const connecting = reconnecting || server.status === "connecting";
  // Click the row to expand its tool list — the "N tools" badge finally has
  // a detail behind it.
  const [open, setOpen] = useState(false);
  const panelId = useId();

  const reconnect = async (): Promise<void> => {
    if (connecting || reconnectingRef.current || server.status === "disabled") return;
    reconnectingRef.current = true;
    setReconnecting(true);
    try {
      await reconnectMCPServer(server.id);
    } catch (cause) {
      if (!mcpServerMutationWasRetired(cause)) {
        notifyError(rpcErrorText(cause) ?? t("tools.reconnectFailed", { server: server.id }));
      }
    } finally {
      reconnectingRef.current = false;
      setReconnecting(false);
    }
  };

  return (
    <div>
      <div className="group grid grid-cols-[40px_1fr_auto_auto_auto] items-center gap-3 px-4 py-3 hover:bg-hover transition-colors">
        <div
          className={cn(
            "grid h-10 w-10 place-items-center rounded-lg bg-surface-2 text-fg-muted group-hover:bg-surface-3 group-hover:text-fg transition-colors",
            server.status === "connected" && "bg-accent-wash text-accent",
            server.status === "failed" && "bg-negative-wash text-negative",
          )}
        >
          <Icon name={server.icon as IconName} size="md" />
        </div>
        {/* The name/desc block is the expand toggle (a nested button inside a
            row-button would be invalid HTML — IconButton sits beside it). */}
        <Pressable
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen((v) => !v)}
          className="min-w-0 border-0 bg-transparent p-0 text-left"
        >
          <div className="text-ui-md font-semibold text-fg truncate">{server.name}</div>
          <div className="mt-0.5 text-ui-md text-fg-faint truncate">{server.desc}</div>
        </Pressable>
        <div className="rounded-sm bg-surface-2 px-1.5 py-0.5 font-mono text-ui-sm text-fg-faint">
          {t("mcp.toolCount", { count: server.tools })}
        </div>
        <div
          className={cn(
            "rounded-sm px-1.5 py-0.5 font-mono text-ui-sm font-semibold",
            pill.classes,
          )}
          title={server.status === "failed" ? server.errorDetail : undefined}
        >
          {t(pill.key)}
        </div>
        <IconButton
          icon="loop"
          iconSize="sm"
          title={t("tools.reconnect")}
          disabled={connecting || server.status === "disabled"}
          onClick={() => void reconnect()}
          className={cn(connecting && "animate-spin")}
        />
      </div>
      {open && (
        <div id={panelId}>
          {server.status === "needsAuth" ? (
            <McpAuthGuide server={server.id} />
          ) : (
            <McpToolList server={server.id} />
          )}
        </div>
      )}
    </div>
  );
}
