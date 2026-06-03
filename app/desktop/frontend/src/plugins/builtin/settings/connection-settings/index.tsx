// Built-in plugin: Connection settings pane.
//
// Owns the `api.baseUrl` config key — both the persistence (mirrored
// into per-plugin storage) and the settings-pane UI. Plain
// `host.config` is in-memory, so the plugin hydrates on setup +
// rewrites on every change so the URL survives across launches.

import { useState } from "react";
import { z } from "zod";
import { RUNTIME_BASE } from "@/main/config";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin, getConfig, setConfig } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

const CONFIG_KEY = "api.baseUrl";
const STORAGE_KEY = "api.baseUrl";

// Backend URL must be a real http(s) origin. Anything else (file://,
// trailing path, plain text) would silently break ky's baseUrl handling
// at the next request — reject on input instead.
const UrlSchema = z.url().refine((v) => v.startsWith("http://") || v.startsWith("https://"), {
  message: "Must start with http:// or https://",
});

function ConnectionPane() {
  const t = useT();
  const initial = (getConfig<string>(CONFIG_KEY) ?? RUNTIME_BASE) || RUNTIME_BASE;
  const [url, setUrl] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  const trimmed = url.trim();
  const dirty = trimmed !== initial.trim();
  const isDefault = trimmed === RUNTIME_BASE;

  const apply = () => {
    // Empty input → silently fall back to the default. Anything else
    // must parse as a real http(s) URL.
    if (!trimmed) {
      setConfig(CONFIG_KEY, RUNTIME_BASE);
      setUrl(RUNTIME_BASE);
      setError(null);
      return;
    }
    const result = UrlSchema.safeParse(trimmed);
    if (!result.success) {
      setError(result.error.issues[0]?.message ?? "Invalid URL");
      return;
    }
    setConfig(CONFIG_KEY, result.data);
    setUrl(result.data);
    setError(null);
  };

  const reset = () => {
    setUrl(RUNTIME_BASE);
    setConfig(CONFIG_KEY, RUNTIME_BASE);
    setError(null);
  };

  return (
    <div>
      <div className="grid grid-cols-[140px_1fr] items-start gap-4 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">{t("settings.connection.title")}</div>
          <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.connection.sub")}</div>
        </div>
        <div className="grid gap-2">
          <label htmlFor="runtime-base-url" className="text-[12px] font-semibold text-fg-faint">
            {t("settings.connection.url")}
          </label>
          <div className="flex items-center gap-2">
            <input
              id="runtime-base-url"
              type="text"
              aria-label={t("settings.connection.url")}
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
              placeholder={RUNTIME_BASE}
              className={cn(
                "flex-1 h-9 rounded-md border bg-surface px-3 font-mono text-[13px] text-fg outline-none",
                error
                  ? "border-negative focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-negative)_18%,transparent)]"
                  : "border-line focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]",
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
          {error ? (
            <div className="text-[11.5px] text-negative">{error}</div>
          ) : dirty ? (
            <div className="text-[11.5px] text-fg-faint">
              {/* Inline hint — applied on blur or Enter. */}↵ to apply · click outside to apply
            </div>
          ) : null}
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

    host.extensions.contribute(SETTINGS_PANE, {
      id: "connection",
      label: "Connection",
      icon: "globe",
      order: 5,
      component: ConnectionPane,
    });
  },
});
