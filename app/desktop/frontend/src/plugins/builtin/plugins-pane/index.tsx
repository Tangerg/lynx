// Built-in plugin: "Plugins" settings pane.
//
// Lists every loaded plugin with name + version + origin + error count.
// Each row has a Reload action; errored rows surface a Clear-errors
// button. Built-ins can be reloaded but not unloaded (they're shipped
// in the bundle; disabling would brick the kernel).

import { useState } from "react";
import { Icon, IconButton, PillButton } from "@/components/common";
import {
  definePlugin,
  reloadPlugin,
  usePluginErrorStore,
  usePluginStore,
} from "@/plugins/sdk";
import { pluginOrigin } from "@/plugins/sideload";

function PluginsPane() {
  const loaded = usePluginStore((s) => s.loaded);
  const log = usePluginErrorStore((s) => s.log);
  const clearFor = usePluginErrorStore((s) => s.clearFor);
  const [reloading, setReloading] = useState<string | null>(null);

  const errorsByPlugin = new Map<string, number>();
  for (const err of log) {
    errorsByPlugin.set(err.plugin, (errorsByPlugin.get(err.plugin) ?? 0) + 1);
  }

  // Sort: built-ins first (alphabetical), then sideloaded (alphabetical).
  // Within each origin group, errored plugins float to the top.
  const rows = Array.from(loaded.values()).sort((a, b) => {
    const oa = pluginOrigin(a.spec.name);
    const ob = pluginOrigin(b.spec.name);
    if (oa !== ob) return oa === "builtin" ? -1 : 1;
    const ea = errorsByPlugin.get(a.spec.name) ?? 0;
    const eb = errorsByPlugin.get(b.spec.name) ?? 0;
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

  return (
    <div>
      <div className="plugin-list">
        {rows.map(({ spec }) => {
          const errCount = errorsByPlugin.get(spec.name) ?? 0;
          const origin = pluginOrigin(spec.name);
          const busy = reloading === spec.name;
          return (
            <div
              key={spec.name}
              className={`plugin-list-row ${errCount > 0 ? "has-errors" : ""}`}
            >
              <div>
                <div className="plugin-list-name">
                  {spec.name}
                  <OriginBadge origin={origin} />
                </div>
                <div className="plugin-list-version">v{spec.version}</div>
                {errCount > 0 && (
                  <div className="plugin-list-errors" style={{ marginTop: 6 }}>
                    <Icon name="bug" size={11} />
                    {errCount} error{errCount === 1 ? "" : "s"} — see browser console
                  </div>
                )}
              </div>
              <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                {errCount > 0 && (
                  <PillButton variant="outlined" size="sm" onClick={() => clearFor(spec.name)}>
                    Clear
                  </PillButton>
                )}
                <IconButton
                  title={busy ? "Reloading…" : "Reload plugin"}
                  onClick={() => handleReload(spec.name)}
                  disabled={busy}
                >
                  <Icon name="loop" size={13} />
                </IconButton>
              </div>
            </div>
          );
        })}
      </div>

      <div style={{
        marginTop: 16,
        color: "var(--color-text-faint)",
        fontSize: 11.5,
        lineHeight: 1.55,
      }}>
        Sideload by dropping a plugin folder containing <code style={codeStyle}>index.js</code>
        {" "}into <code style={codeStyle}>~/.lyra/plugins/</code> and restarting the app.
        See <code style={codeStyle}>frontend/sample-plugins/hello-sideload/</code> for a template.
      </div>
    </div>
  );
}

function OriginBadge({ origin }: { origin: "builtin" | "sideload" }) {
  return (
    <span
      className={`plugin-origin-badge plugin-origin-${origin}`}
      title={origin === "builtin" ? "Ships with Lyra" : "User-installed"}
    >
      {origin === "builtin" ? "Built-in" : "Sideload"}
    </span>
  );
}

const codeStyle: React.CSSProperties = {
  fontFamily: "var(--font-mono)",
  background: "var(--color-surface-2)",
  padding: "1px 5px",
  borderRadius: 3,
  color: "var(--color-text)",
};

export default definePlugin({
  name: "lyra.builtin.plugins-pane",
  version: "1.0.0",
  setup({ host }) {
    host.settings.registerPane({
      id: "plugins",
      label: "Plugins",
      icon: "tool",
      order: 99,
      component: PluginsPane,
    });
  },
});
