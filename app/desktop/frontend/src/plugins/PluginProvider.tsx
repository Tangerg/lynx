import { useEffect, useState, type ReactNode } from "react";
import { builtinPlugins } from "./builtin";
import { installHostBridge } from "./hostBridge";
import { loadPlugins, usePluginStore } from "./sdk";
import { loadSideloadedPlugins, tagAllAsBuiltin } from "./sideload";

type Props = {
  children: ReactNode;
};

/**
 * PluginProvider — startup orchestrator for plugins.
 *
 *   1. Install the `window.__LYRA__` bridge so sideloaded modules can reach
 *      the host's React / motion / SDK without bundling their own.
 *   2. Load built-in plugins (sync — already in the bundle).
 *   3. Tag everything loaded so far as "builtin" for the Plugins pane.
 *   4. Fetch the sideload manifest from the Go backend and dynamic-import
 *      each plugin.
 *
 * Children are gated on the *built-in* plugins finishing because anything
 * that lives in the bundle's startup path (routes, layout slots, themes) is
 * a built-in plugin contribution. Sideloaded plugins load in the background
 * and add to the registry as they arrive — they're not on the critical path.
 *
 * Built-in setup is synchronous (no I/O), so the gate resolves on the next
 * microtask — visible as nothing more than the first paint blanking briefly.
 */
export function PluginProvider({ children }: Props) {
  const [builtinsReady, setBuiltinsReady] = useState(false);

  useEffect(() => {
    let cancelled = false;

    // Bridge is sync (static imports). Install before anything else so
    // sideloaded plugins that touch window.__LYRA__ at module-evaluation
    // time can see it.
    installHostBridge();

    void (async () => {
      await loadPlugins(builtinPlugins);
      tagAllAsBuiltin();
      // Flush lifecycle.onReady listeners — plugins that registered
      // onReady at setup time get called now (in registration order).
      usePluginStore.getState().markAppReady();
      if (!cancelled) setBuiltinsReady(true);
      // Sideloaded plugins kick off independently — they're additive and
      // don't block the kernel.
      void loadSideloadedPlugins();
    })();

    return () => { cancelled = true; };
  }, []);

  // Nothing to show until built-ins register (router has no routes, layout
  // slots are empty). One tick of blank is preferable to a flash of "no
  // routes match" or an empty kernel.
  if (!builtinsReady) return null;

  return <>{children}</>;
}
