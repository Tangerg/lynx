import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { TooltipProvider } from "@/ui";
import { builtinPlugins } from "../builtin";
import { startKernel, stopKernel } from "../sdk";
import { retractKernel } from "../sdk/kernel";
import { publishHostBridge } from "./hostBridge";
import { loadSideloadedPlugins, type SideloadDiscovery } from "./sideloadDiscovery";
import type { Host } from "dougong";

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
    const controller = new AbortController();
    let retired = false;
    let host: Host | undefined;
    let sideloads: SideloadDiscovery | undefined;
    let disposal: Promise<void> | undefined;

    const disposeOwnedResources = () => {
      if (!host || disposal) return disposal;
      const ownedHost = host;
      const ownedSideloads = sideloads;
      // Retire the read side synchronously. Window unload cannot await, and an
      // old renderer must stop being actionable before async resource joins.
      retractKernel(ownedHost);
      disposal = (async () => {
        let sideloadError: unknown;
        try {
          await ownedSideloads?.dispose();
        } catch (error) {
          sideloadError = error;
        }
        try {
          await stopKernel(ownedHost);
        } catch (hostError) {
          if (sideloadError) {
            throw new AggregateError(
              [sideloadError, hostError],
              "Sideload and kernel teardown both failed",
            );
          }
          throw hostError;
        }
        if (sideloadError) throw sideloadError;
      })().catch((error: unknown) => {
        console.error("[plugin] kernel teardown failed:", error);
      });
      return disposal;
    };

    publishHostBridge();

    void (async () => {
      try {
        host = await startKernel(builtinPlugins, controller.signal);
        if (retired) {
          void disposeOwnedResources();
          return;
        }
        setReady(true);
        sideloads = loadSideloadedPlugins(host);
        void sideloads.completion;
      } catch (error) {
        if (!retired) console.error("[plugin] kernel startup failed:", error);
      }
    })();

    return () => {
      retired = true;
      controller.abort();
      void disposeOwnedResources();
    };
  }, []);

  if (!ready) return null;

  return <TooltipProvider>{children}</TooltipProvider>;
}
