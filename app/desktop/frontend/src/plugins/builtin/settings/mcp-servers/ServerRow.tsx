import { useState } from "react";
import { Icon, PillButton, StatusDot, Switch } from "@/components/common";
import {
  type MCPServerConfig,
  type MCPServerTransport,
  useAuthorizeMCPServer,
  useSetMCPServerEnabled,
} from "./application/mcpServerConfig";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { ServerForm } from "./ServerForm";

const STATUS_TONE: Record<
  NonNullable<MCPServerConfig["status"]>,
  "ok" | "running" | "waiting" | "err" | "idle"
> = {
  connected: "ok",
  connecting: "running",
  needsAuth: "waiting",
  failed: "err",
  disconnected: "idle",
};

function TransportBadge({ transport }: { transport: MCPServerTransport }) {
  return (
    <span className="rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-fg-muted">
      {transport}
    </span>
  );
}

export function ServerRow({ server }: { server: MCPServerConfig }) {
  const t = useT();
  const setEnabled = useSetMCPServerEnabled();
  const authorize = useAuthorizeMCPServer();
  const [editing, setEditing] = useState(false);

  const onToggle = async (enabled: boolean) => {
    try {
      await setEnabled(server.name, enabled);
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("mcp.error.toggle"), { source: "mcp" });
    }
  };

  const onSignIn = async () => {
    try {
      await authorize(server.name);
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("mcp.error.signIn"), { source: "mcp" });
    }
  };

  const tone = server.status ? STATUS_TONE[server.status] : "idle";
  const active = server.status === "connected";

  return (
    <div className="rounded-lg border border-line-soft bg-canvas px-3 py-2.5">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
        <StatusDot tone={tone} />
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[14px] font-semibold text-fg" title={server.name}>
            {server.name}
          </span>
          <TransportBadge transport={server.type} />
          {server.status === "failed" && server.errorDetail && (
            <span className="truncate text-[11px] text-negative" title={server.errorDetail}>
              {server.errorDetail}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2.5">
          {active && (
            <span className="font-mono text-[11px] tabular-nums text-fg-faint">
              {t("mcp.toolCount", { count: server.toolCount ?? 0 })}
            </span>
          )}
          {server.status === "needsAuth" && (
            <PillButton variant="accent" size="sm" onClick={() => void onSignIn()}>
              {t("mcp.signIn")}
            </PillButton>
          )}
          <Switch
            checked={server.enabled}
            onCheckedChange={(value) => void onToggle(value)}
            ariaLabel={t("mcp.enable.aria", { server: server.name })}
          />
          <button
            type="button"
            aria-label={t("mcp.edit", { server: server.name })}
            aria-expanded={editing}
            onClick={() => setEditing((value) => !value)}
            className="grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="edit" size={13} />
          </button>
        </div>
      </div>

      {editing && (
        <div className="mt-2.5">
          <ServerForm
            server={server}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}
