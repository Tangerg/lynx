import { useEffect, useId, useRef, useState } from "react";
import { IconButton, PillButton, StatusDot, Switch } from "@/ui";
import {
  type MCPServerSettings,
  type MCPTransport,
  mcpServerMutationWasRetired,
  useAuthorizeMCPServer,
  useSetMCPServerEnabled,
} from "../application/mcpServerConfig";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { ServerForm } from "./ServerForm";

const STATUS_TONE: Record<
  MCPServerSettings["status"],
  "ok" | "running" | "waiting" | "err" | "idle"
> = {
  disabled: "idle",
  connected: "ok",
  connecting: "running",
  needsAuth: "waiting",
  failed: "err",
  disconnected: "idle",
};

function TransportBadge({ transport }: { transport: MCPTransport }) {
  return (
    <span className="rounded-sm bg-surface-2 px-1.5 py-0.5 font-mono text-ui-sm text-fg-muted">
      {transport}
    </span>
  );
}

export function ServerRow({ server }: { server: MCPServerSettings }) {
  const t = useT();
  const setEnabled = useSetMCPServerEnabled();
  const authorize = useAuthorizeMCPServer();
  const [editing, setEditing] = useState(false);
  const panelId = useId();
  const [signingIn, setSigningIn] = useState(false);
  const authorizationController = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      const controller = authorizationController.current;
      authorizationController.current = null;
      controller?.abort();
    },
    [],
  );

  const onToggle = async (enabled: boolean) => {
    try {
      await setEnabled(server.name, enabled);
    } catch (err) {
      if (mcpServerMutationWasRetired(err)) return;
      notifyError(err instanceof Error ? err.message : t("mcp.error.toggle"), { source: "mcp" });
    }
  };

  const onSignIn = async () => {
    const controller = new AbortController();
    authorizationController.current?.abort();
    authorizationController.current = controller;
    setSigningIn(true);
    try {
      await authorize(server.name, controller.signal);
    } catch (err) {
      if (controller.signal.aborted || mcpServerMutationWasRetired(err)) return;
      notifyError(err instanceof Error ? err.message : t("mcp.error.signIn"), { source: "mcp" });
    } finally {
      if (authorizationController.current === controller) {
        authorizationController.current = null;
        setSigningIn(false);
      }
    }
  };

  const tone = STATUS_TONE[server.status];
  const active = server.status === "connected";

  return (
    <div className="rounded-md px-3 py-2.5 transition-colors hover:bg-hover">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
        <StatusDot tone={tone} />
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-ui-md font-medium text-fg" title={server.name}>
            {server.name}
          </span>
          <TransportBadge transport={server.type} />
          {server.status === "failed" && server.errorDetail && (
            <span className="truncate text-ui-md text-negative" title={server.errorDetail}>
              {server.errorDetail}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2.5">
          {active && (
            <span className="font-mono text-ui-md tabular-nums text-fg-muted">
              {t("mcp.toolCount", { count: server.toolCount ?? 0 })}
            </span>
          )}
          {(server.status === "needsAuth" || signingIn) && (
            <PillButton
              variant="accent"
              size="sm"
              disabled={signingIn}
              onClick={() => void onSignIn()}
            >
              {t(signingIn ? "mcp.signingIn" : "mcp.signIn")}
            </PillButton>
          )}
          <Switch
            checked={server.enabled}
            onCheckedChange={(value) => void onToggle(value)}
            ariaLabel={t("mcp.enable.aria", { server: server.name })}
          />
          <IconButton
            icon="edit"
            size="sm"
            iconSize="sm"
            active={editing}
            title={t("mcp.edit", { server: server.name })}
            aria-expanded={editing}
            aria-controls={panelId}
            onClick={() => setEditing((value) => !value)}
          />
        </div>
      </div>

      {editing && (
        <div id={panelId} className="mt-2.5">
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
