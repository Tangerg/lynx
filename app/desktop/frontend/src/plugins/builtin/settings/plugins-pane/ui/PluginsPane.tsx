// The "Plugins" settings pane.
//
// Lists every installed plugin with name + origin + error count.
// Errored rows expand inline to show each error's source, message, and
// stack (captured at the catch site, see sdk/errors.ts) so a broken
// plugin is debuggable without opening the browser console. Errored rows
// surface a Clear-errors button.
//
// No per-row reload: a built-in's installation is part of one boot transaction
// and reinstalling it alone would leave the graph in a state the Host never
// agreed to. Sideloaded plugins are removed and re-registered by the platform.

import { Trans } from "@/lib/i18n";
import type { PluginError, PluginErrorSource } from "@/plugins/sdk";
import { useState } from "react";
import { Icon, IconButton, PillButton, TextButton } from "@/ui";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useInstalledPlugins, usePluginErrorStore } from "@/plugins/sdk";
import { pluginOrigin } from "@/plugins/sdk/pluginOrigin";

export function PluginsPane() {
  const t = useT();
  const installed = useInstalledPlugins();
  const log = usePluginErrorStore((s) => s.log);
  const clearFor = usePluginErrorStore((s) => s.clearFor);
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
  const rows = [...installed].sort((a, b) => {
    const oa = pluginOrigin(a);
    const ob = pluginOrigin(b);
    if (oa !== ob) return oa === "builtin" ? -1 : 1;
    const ea = errorsByPlugin.get(a)?.length ?? 0;
    const eb = errorsByPlugin.get(b)?.length ?? 0;
    if (ea !== eb) return eb - ea;
    return a.localeCompare(b);
  });

  const toggle = (name: string) =>
    setExpanded((cur) => {
      const next = new Set(cur);
      if (!next.delete(name)) next.add(name);
      return next;
    });

  return (
    <div>
      <div className="flex flex-col gap-2">
        {rows.map((name) => {
          const errors = errorsByPlugin.get(name) ?? [];
          const errCount = errors.length;
          const origin = pluginOrigin(name);
          const open = expanded.has(name);
          return (
            <div
              key={name}
              className={cn(
                "rounded-md transition-colors hover:bg-hover",
                errCount > 0 && "bg-negative-wash",
              )}
            >
              <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2.5 px-3 py-2.5">
                <div>
                  <div className="text-ui-md font-medium text-fg">
                    {name}
                    <OriginBadge origin={origin} />
                  </div>
                  {errCount > 0 && (
                    <TextButton
                      tone="negative"
                      onClick={() => toggle(name)}
                      title={open ? t("plugins.errorDetail.hide") : t("plugins.errorDetail.show")}
                      className="mt-1.5"
                    >
                      <Icon name="bug" size="xs" />
                      {t("plugins.errors", { count: errCount })}
                      <Icon name={open ? "chevron-up" : "chevron-down"} size="xs" />
                    </TextButton>
                  )}
                </div>
                <div className="flex items-center gap-1.5">
                  {errCount > 0 && (
                    <PillButton variant="outlined" size="sm" onClick={() => clearFor(name)}>
                      {t("plugins.clear")}
                    </PillButton>
                  )}
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

      <div className="mt-4 text-ui-md leading-body text-fg-muted">
        <Trans
          i18nKey="plugins.sideload"
          values={{
            file: "index.js",
            dir: "~/.lyra/plugins/",
            sample: "frontend/sample-plugins/hello-sideload/",
          }}
          components={{ code: <code className={INLINE_CODE} /> }}
        />
      </div>
    </div>
  );
}

const INLINE_CODE = "rounded-2xs bg-surface-2 px-1.5 py-px font-mono text-fg";

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
    <div className="rounded-md bg-sunken px-2.5 py-2">
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2">
        <span className="rounded-full bg-negative-badge px-1.5 py-px font-mono text-ui-xs font-semibold text-negative">
          {SOURCE_LABEL[err.source]}
        </span>
        <span className="truncate font-medium text-ui-md text-fg" title={err.message}>
          {err.message}
        </span>
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-ui-xs text-fg-faint">{time}</span>
          <IconButton icon="copy" iconSize="xs" title={t("plugins.copyError")} onClick={copy} />
        </div>
      </div>
      {err.detail && (
        <pre className="mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words font-mono text-ui-sm leading-body text-fg-muted">
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
        "ml-2 inline-block rounded-full px-1.5 py-px font-mono text-ui-xs font-semibold align-middle tracking-normal",
        origin === "builtin" ? "bg-surface-2 text-fg-muted" : "bg-info-wash text-info",
      )}
    >
      {origin === "builtin" ? t("plugins.origin.builtin") : t("plugins.origin.sideload")}
    </span>
  );
}
