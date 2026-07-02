// Built-in plugin: Connection settings pane.
//
// Owns the `api.baseUrl` config key — both the persistence (mirrored
// into per-plugin storage) and the settings-pane UI. Plain
// `host.config` is in-memory, so the plugin hydrates on setup +
// rewrites on every change so the URL survives across launches. The UI
// itself lives in ui/ConnectionPane.

import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { RUNTIME_BASE_CONFIG_KEY, RUNTIME_BASE_STORAGE_KEY } from "./application/runtimeConnection";
import { ConnectionPane } from "./ui/ConnectionPane";

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  version: "1.0.0",
  setup({ host }) {
    // Hydrate persisted URL into host.config so the first ky request
    // (made before any UI mounts) already sees the user's choice.
    const stored = host.storage.get<string>(RUNTIME_BASE_STORAGE_KEY);
    if (typeof stored === "string" && stored) {
      host.config.set(RUNTIME_BASE_CONFIG_KEY, stored);
    }

    // Persist any future change. host.config dedupes by Object.is so
    // setting the same value twice is a no-op.
    host.config.onChange(RUNTIME_BASE_CONFIG_KEY, (value) => {
      if (typeof value === "string") host.storage.set(RUNTIME_BASE_STORAGE_KEY, value);
    });

    host.extensions.contribute(SETTINGS_PANE, {
      id: "connection",
      label: "settings.pane.connection",
      group: "general",
      icon: "globe",
      order: 5,
      component: ConnectionPane,
    });
  },
});
