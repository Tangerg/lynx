import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { TooltipProvider } from "@/ui";
import { builtinPlugins } from "../builtin";
import { startKernel } from "../sdk";
import { publishHostBridge } from "./hostBridge";
import { loadSideloadedPlugins } from "./sideloadDiscovery";

interface Props {
  children: ReactNode;
}

/**
 * Startup: bridge first (a sideloaded module can touch `window.__LYRA__` at
 * module-evaluation time), then the built-in kernel, then sideloads in the
 * background.
 *
 * Children wait on the kernel because everything in the startup path — routes,
 * layout slots, themes — is a built-in contribution. One blank tick beats a
 * flash of "no routes match".
 */
export function PluginProvider({ children }: Props) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    publishHostBridge();

    void (async () => {
      await startKernel(builtinPlugins);
      if (cancelled) return;
      setReady(true);
      void loadSideloadedPlugins();
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  if (!ready) return null;

  return <TooltipProvider>{children}</TooltipProvider>;
}
