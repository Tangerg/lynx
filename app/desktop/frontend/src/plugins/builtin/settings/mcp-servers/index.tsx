// Built-in plugin: "MCP servers" settings pane. The editable MCP registry
// (workspace.mcp.listConfigs) — add / edit / remove servers, toggle their
// enablement, and paste a Claude-Desktop JSON block to bulk-import. Mirrors
// the Providers pane's list + per-row save/test/probe shape; the live status
// dot folds the best-effort connection state the runtime reports per entry.
//
// Per-tool gating (disabledTools / autoApproveTools) is carried by the wire
// config but not exposed here yet — its enforcement (hiding a tool from the
// model / skipping the approval gate) threads through the tool resolver and
// the safety-critical approval gate, landing as a dedicated follow-up.

import type { MCPServerConfigInfo, MCPTransport } from "@/lib/data/queries";
import { useState } from "react";
import { DataView, Icon, PillButton, StatusDot, Switch } from "@/components/common";
import { useMCPConfigs } from "@/lib/data/queries";
import { useConfigureMCPServer, useSetMCPServerEnabled } from "@/lib/agent/useMCPServerConfig";
import { notifyError, notifyInfo } from "@/lib/notify";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ServerForm } from "./ServerForm";
import { parseMcpImport } from "./mcpImport";

// Live-status dot tone — folds the 5-state McpStatus onto the StatusDot ramp
// (connecting pulses like a running agent; connected = success green).
const STATUS_TONE: Record<
  NonNullable<MCPServerConfigInfo["status"]>,
  "ok" | "running" | "waiting" | "err" | "idle"
> = {
  connected: "ok",
  connecting: "running",
  needsAuth: "waiting",
  failed: "err",
  disconnected: "idle",
};

function TransportBadge({ transport }: { transport: MCPTransport }) {
  return (
    <span className="rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-fg-muted">
      {transport}
    </span>
  );
}

function ServerRow({ s }: { s: MCPServerConfigInfo }) {
  const t = useT();
  const setEnabled = useSetMCPServerEnabled();
  const [editing, setEditing] = useState(false);

  const onToggle = async (enabled: boolean) => {
    try {
      await setEnabled(s.name, enabled);
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("mcp.error.toggle"), { source: "mcp" });
    }
  };

  const tone = s.status ? STATUS_TONE[s.status] : "idle";
  const active = s.status === "connected";

  return (
    <div className="rounded-lg border border-line-soft bg-canvas px-3 py-2.5">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
        <StatusDot tone={tone} />
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[14px] font-semibold text-fg" title={s.name}>
            {s.name}
          </span>
          <TransportBadge transport={s.transport} />
          {s.status === "failed" && s.errorDetail && (
            <span className="truncate text-[11px] text-negative" title={s.errorDetail}>
              {s.errorDetail}
            </span>
          )}
        </div>
        <div className="flex items-center gap-2.5">
          {active && (
            <span className="font-mono text-[11px] tabular-nums text-fg-faint">
              {t("mcp.toolCount", { count: s.toolCount ?? 0 })}
            </span>
          )}
          <Switch
            checked={s.enabled}
            onCheckedChange={(v) => void onToggle(v)}
            ariaLabel={t("mcp.enable.aria", { server: s.name })}
          />
          <button
            type="button"
            aria-label={t("mcp.edit", { server: s.name })}
            aria-expanded={editing}
            onClick={() => setEditing((v) => !v)}
            className="grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="edit" size={13} />
          </button>
        </div>
      </div>

      {editing && (
        <div className="mt-2.5">
          <ServerForm
            server={s}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}

function JsonImport() {
  const t = useT();
  const configure = useConfigureMCPServer();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();

  const onImport = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const { configs } = parseMcpImport(text);
      for (const c of configs) await configure(c);
      notifyInfo(t("mcp.import.ok", { count: configs.length }), { source: "mcp" });
      setText("");
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("mcp.import.error"));
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex items-center gap-1.5 text-[12px] text-fg-muted hover:text-fg"
      >
        <Icon name="download" size={13} />
        {t("mcp.import")}
      </button>
    );
  }
  return (
    <div className="flex flex-col gap-2 rounded-md bg-surface-2 p-3">
      <span className="text-[11px] text-fg-muted">{t("mcp.import.hint")}</span>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={6}
        spellCheck={false}
        aria-label={t("mcp.import.hint")}
        placeholder={'{"mcpServers": {"my-server": {"command": "npx", "args": ["..."]}}}'}
        className="w-full resize-y rounded-md border border-line-soft bg-surface px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-fg outline-none placeholder:text-fg-faint focus:border-accent"
      />
      {error && (
        <span className="inline-flex items-center gap-1 text-[12px] text-negative">
          <Icon name="alert" size={13} />
          <span className="truncate" title={error}>
            {error}
          </span>
        </span>
      )}
      <div className="flex items-center gap-2">
        <PillButton
          variant="accent"
          size="sm"
          disabled={!text.trim() || busy}
          onClick={() => void onImport()}
        >
          {busy ? t("mcp.importing") : t("mcp.import.confirm")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={() => setOpen(false)}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </div>
  );
}

function McpServersPane() {
  const t = useT();
  const { data, isLoading, isError } = useMCPConfigs();
  const [adding, setAdding] = useState(false);

  return (
    <div className="flex flex-col gap-3">
      <div className={cn("flex items-center justify-between gap-3", adding && "items-start")}>
        {adding ? (
          <div className="flex-1">
            <ServerForm onDone={() => setAdding(false)} onCancel={() => setAdding(false)} />
          </div>
        ) : (
          <>
            <JsonImport />
            <PillButton variant="outlined" size="sm" onClick={() => setAdding(true)}>
              <Icon name="plus" size={13} />
              {t("mcp.add")}
            </PillButton>
          </>
        )}
      </div>

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "tool",
          title: t("mcp.empty"),
          sub: t("mcp.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((s) => (
              <ServerRow key={s.name} s={s} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "mcp-servers",
      label: "settings.pane.mcpServers",
      icon: "tool",
      // After Providers (50) and Approvals (55) — both are "what the agent can
      // reach"; MCP servers extend that surface.
      order: 56,
      component: McpServersPane,
    });
  },
});
