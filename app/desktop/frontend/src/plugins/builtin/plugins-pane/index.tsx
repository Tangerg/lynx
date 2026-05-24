// Built-in plugin: "Plugins" settings pane.
//
// Lists every loaded plugin with name + version + origin + error count.
// Each row has a Reload action; errored rows surface a Clear-errors
// button. Built-ins can be reloaded but not unloaded (they're shipped
// in the bundle; disabling would brick the kernel).

import { useState } from "react";
import { Icon, IconButton, PillButton } from "@/components/common";
import { cn } from "@/lib/utils";
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
      <div className="flex flex-col gap-2">
        {rows.map(({ spec }) => {
          const errCount = errorsByPlugin.get(spec.name) ?? 0;
          const origin = pluginOrigin(spec.name);
          const busy = reloading === spec.name;
          return (
            <div
              key={spec.name}
              className={cn(
                "grid grid-cols-[1fr_auto] gap-2.5 rounded-lg border border-line-soft bg-canvas px-3 py-2.5",
                errCount > 0 && "border-[rgba(243,114,127,0.36)]",
              )}
            >
              <div>
                <div className="text-[13px] font-semibold text-fg">
                  {spec.name}
                  <OriginBadge origin={origin} />
                </div>
                <div className="font-mono text-[11px] text-fg-faint">v{spec.version}</div>
                {errCount > 0 && (
                  <div className="mt-1.5 inline-flex items-center gap-1.5 text-[11px] text-negative">
                    <Icon name="bug" size={11} />
                    {errCount} error{errCount === 1 ? "" : "s"} — see browser console
                  </div>
                )}
              </div>
              <div className="flex items-center gap-1.5">
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

      <div className="mt-4 text-[11.5px] leading-[1.55] text-fg-faint">
        Sideload by dropping a plugin folder containing <code className={INLINE_CODE}>index.js</code>
        {" "}into <code className={INLINE_CODE}>~/.lyra/plugins/</code> and restarting the app.
        See <code className={INLINE_CODE}>frontend/sample-plugins/hello-sideload/</code> for a template.
      </div>
    </div>
  );
}

const INLINE_CODE = "rounded-[3px] bg-surface-2 px-1.5 py-px font-mono text-fg";

function OriginBadge({ origin }: { origin: "builtin" | "sideload" }) {
  return (
    <span
      title={origin === "builtin" ? "Ships with Lyra" : "User-installed"}
      className={cn(
        "ml-2 inline-block rounded-full px-1.5 py-px font-mono text-[10px] font-semibold align-middle tracking-normal",
        origin === "builtin"
          ? "bg-surface-2 text-fg-muted"
          : "bg-[rgba(82,157,245,0.14)] text-info",
      )}
    >
      {origin === "builtin" ? "Built-in" : "Sideload"}
    </span>
  );
}

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
