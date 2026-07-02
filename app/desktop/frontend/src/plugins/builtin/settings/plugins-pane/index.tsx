// Built-in plugin: "Plugins" settings pane.
//
// Lists every loaded plugin with name + version + origin + error count.
// Errored rows expand inline to show each error's source, message, and
// stack (captured at the catch site, see sdk/errors.ts) so a broken
// plugin is debuggable without opening the browser console. Each row has
// a Reload action; errored rows surface a Clear-errors button. Built-ins
// can be reloaded but not unloaded (they're shipped in the bundle;
// disabling would brick the kernel).

import type { PluginError, PluginErrorSource } from "@/plugins/sdk";
import { useState } from "react";
import { Icon, IconButton, PillButton } from "@/components/common";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin, reloadPlugin, usePluginErrorStore, usePluginStore } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { pluginOrigin } from "@/plugins/host/sideload";

function PluginsPane() {
  const t = useT();
  const loaded = usePluginStore((s) => s.loaded);
  const log = usePluginErrorStore((s) => s.log);
  const clearFor = usePluginErrorStore((s) => s.clearFor);
  const [reloading, setReloading] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  // Newest-first list per plugin (the count is the list length).
  const errorsByPlugin = new Map<string, PluginError[]>();
  for (const err of log) {
    const list = errorsByPlugin.get(err.plugin);
    if (list) list.unshift(err);
    else errorsByPlugin.set(err.plugin, [err]);
  }

  // Sort: built-ins first (alphabetical), then sideloaded (alphabetical).
  // Within each origin group, errored plugins float to the top.
  const rows = Array.from(loaded.values()).sort((a, b) => {
    const oa = pluginOrigin(a.spec.name);
    const ob = pluginOrigin(b.spec.name);
    if (oa !== ob) return oa === "builtin" ? -1 : 1;
    const ea = errorsByPlugin.get(a.spec.name)?.length ?? 0;
    const eb = errorsByPlugin.get(b.spec.name)?.length ?? 0;
    if (ea !== eb) return eb - ea;
    return a.spec.name.localeCompare(b.spec.name);
  });

  const handleReload = async (name: string) => {
    setReloading(name);
    try {
      await reloadPlugin(name);
    } finally {
      setReloading((cur) => (cur === name ? null : cur));
    }
  };

  const toggle = (name: string) =>
    setExpanded((cur) => {
      const next = new Set(cur);
      if (!next.delete(name)) next.add(name);
      return next;
    });

  return (
    <div>
      <div className="flex flex-col gap-2">
        {rows.map(({ spec }) => {
          const errors = errorsByPlugin.get(spec.name) ?? [];
          const errCount = errors.length;
          const origin = pluginOrigin(spec.name);
          const busy = reloading === spec.name;
          const open = expanded.has(spec.name);
          return (
            <div
              key={spec.name}
              className={cn(
                "rounded-lg bg-canvas",
                errCount > 0 && "border-[0.5px] border-[rgba(243,114,127,0.36)]",
              )}
            >
              <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2.5 px-3 py-2.5">
                <div>
                  <div className="text-[14px] font-semibold text-fg">
                    {spec.name}
                    <OriginBadge origin={origin} />
                  </div>
                  <div className="font-mono text-[12px] text-fg-faint">v{spec.version}</div>
                  {errCount > 0 && (
                    <button
                      type="button"
                      onClick={() => toggle(spec.name)}
                      title={open ? t("plugins.errorDetail.hide") : t("plugins.errorDetail.show")}
                      className="mt-1.5 inline-flex items-center gap-1.5 text-[12px] text-negative hover:opacity-80"
                    >
                      <Icon name="bug" size={12} />
                      {t("plugins.errors", { count: errCount })}
                      <Icon name={open ? "chevron-up" : "chevron-down"} size={12} />
                    </button>
                  )}
                </div>
                <div className="flex items-center gap-1.5">
                  {errCount > 0 && (
                    <PillButton variant="outlined" size="sm" onClick={() => clearFor(spec.name)}>
                      {t("plugins.clear")}
                    </PillButton>
                  )}
                  <IconButton
                    title={busy ? t("plugins.reloading") : t("plugins.reload")}
                    onClick={() => handleReload(spec.name)}
                    disabled={busy}
                  >
                    <Icon name="loop" size={13} />
                  </IconButton>
                </div>
              </div>
              {open && errCount > 0 && (
                <div className="flex flex-col gap-1.5 px-3 pb-3">
                  {errors.map((err) => (
                    <ErrorEntry key={err.id} err={err} />
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      <div className="mt-4 text-[13px] leading-[1.55] text-fg-muted">
        Sideload by dropping a plugin folder containing{" "}
        <code className={INLINE_CODE}>index.js</code> into{" "}
        <code className={INLINE_CODE}>~/.lyra/plugins/</code> and restarting the app. See{" "}
        <code className={INLINE_CODE}>frontend/sample-plugins/hello-sideload/</code> for a template.
      </div>
    </div>
  );
}

const INLINE_CODE = "rounded-[3px] bg-surface-2 px-1.5 py-px font-mono text-fg";

// Where the error was caught (sdk/errors.ts PluginErrorSource).
const SOURCE_LABEL: Record<PluginErrorSource, string> = {
  setup: "setup",
  render: "render",
  events: "event handler",
  command: "command",
  other: "other",
};

function ErrorEntry({ err }: { err: PluginError }) {
  const t = useT();
  const time = new Date(err.timestamp).toLocaleTimeString();
  const copy = () =>
    void copyText(
      `[${SOURCE_LABEL[err.source]}] ${err.message}${err.detail ? `\n\n${err.detail}` : ""}`,
    );
  return (
    <div className="rounded-md bg-surface-2 px-2.5 py-2">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2">
        <span className="rounded-full bg-[rgba(243,114,127,0.16)] px-1.5 py-px font-mono text-[10px] font-semibold text-negative">
          {SOURCE_LABEL[err.source]}
        </span>
        <span className="truncate font-medium text-[12px] text-fg" title={err.message}>
          {err.message}
        </span>
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-[10.5px] text-fg-faint">{time}</span>
          <IconButton title={t("plugins.copyError")} onClick={copy}>
            <Icon name="copy" size={12} />
          </IconButton>
        </div>
      </div>
      {err.detail && (
        <pre className="mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-[1.5] text-fg-muted">
          {err.detail}
        </pre>
      )}
    </div>
  );
}

function OriginBadge({ origin }: { origin: "builtin" | "sideload" }) {
  const t = useT();
  return (
    <span
      title={
        origin === "builtin"
          ? t("plugins.origin.builtin.title")
          : t("plugins.origin.sideload.title")
      }
      className={cn(
        "ml-2 inline-block rounded-full px-1.5 py-px font-mono text-[10px] font-semibold align-middle tracking-normal",
        origin === "builtin"
          ? "bg-surface-2 text-fg-muted"
          : "bg-[rgba(82,157,245,0.14)] text-info",
      )}
    >
      {origin === "builtin" ? t("plugins.origin.builtin") : t("plugins.origin.sideload")}
    </span>
  );
}

export default definePlugin({
  name: "lyra.builtin.plugins-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "plugins",
      label: "settings.pane.plugins",
      group: "integrations",
      icon: "tool",
      order: 99,
      component: PluginsPane,
    });
  },
});
