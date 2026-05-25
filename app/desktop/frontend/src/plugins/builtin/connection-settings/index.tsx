// Built-in plugin: Connection settings pane.
//
// Owns the `api.baseUrl` config key — both the persistence (mirrored
// into per-plugin storage) and the settings-pane UI. Plain
// `host.config` is in-memory, so the plugin hydrates on setup +
// rewrites on every change so the URL survives across launches.

import { useState } from "react";
import { AGUI_BASE } from "@/lib/http";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin, getConfig, setConfig } from "@/plugins/sdk";

const CONFIG_KEY = "api.baseUrl";
const STORAGE_KEY = "api.baseUrl";

function ConnectionPane() {
  const t = useT();
  const initial = (getConfig<string>(CONFIG_KEY) ?? AGUI_BASE) || AGUI_BASE;
  const [url, setUrl] = useState(initial);

  const dirty = url.trim() !== initial.trim();
  const isDefault = url.trim() === AGUI_BASE;

  const apply = () => {
    const next = url.trim() || AGUI_BASE;
    setConfig(CONFIG_KEY, next);
    // The plugin's onChange listener (see setup below) mirrors this to
    // localStorage so the next launch reads it back.
    setUrl(next);
  };

  const reset = () => {
    setUrl(AGUI_BASE);
    setConfig(CONFIG_KEY, AGUI_BASE);
  };

  return (
    <div>
      <div className="grid grid-cols-[140px_1fr] items-start gap-4 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">{t("settings.connection.title")}</div>
          <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.connection.sub")}</div>
        </div>
        <div className="grid gap-2">
          <label className="text-[12px] font-semibold text-fg-faint">
            {t("settings.connection.url")}
          </label>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onBlur={apply}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  apply();
                  (e.target as HTMLInputElement).blur();
                }
              }}
              placeholder={AGUI_BASE}
              className={cn(
                "flex-1 h-9 rounded-md border border-line bg-surface px-3 font-mono text-[13px] text-fg outline-none",
                "focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]",
              )}
              spellCheck={false}
            />
            {!isDefault && (
              <button
                type="button"
                onClick={reset}
                className="h-9 shrink-0 rounded-md border border-line bg-transparent px-3 font-sans text-[12.5px] text-fg-muted cursor-pointer hover:bg-surface-3 hover:text-fg transition-colors"
              >
                {t("settings.connection.reset")}
              </button>
            )}
          </div>
          {dirty && (
            <div className="text-[11.5px] text-fg-faint">
              {/* Inline hint — applied on blur or Enter. */}
              ↵ to apply · click outside to apply
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  version: "1.0.0",
  setup({ host }) {
    // Hydrate persisted URL into host.config so the first ky request
    // (made before any UI mounts) already sees the user's choice.
    const stored = host.storage.get<string>(STORAGE_KEY);
    if (typeof stored === "string" && stored) {
      host.config.set(CONFIG_KEY, stored);
    }

    // Persist any future change. host.config dedupes by Object.is so
    // setting the same value twice is a no-op.
    host.config.onChange(CONFIG_KEY, (value) => {
      if (typeof value === "string") host.storage.set(STORAGE_KEY, value);
    });

    host.settings.registerPane({
      id: "connection",
      label: "Connection",
      icon: "globe",
      order: 5,
      component: ConnectionPane,
    });
  },
});
