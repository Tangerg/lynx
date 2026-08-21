import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { TooltipProvider } from "@/ui";
import { builtinPlugins } from "../builtin";
import { startKernel, stopKernel } from "../sdk";
import type { Host } from "dougong";

interface Props {
  children: ReactNode;
}

/**
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
    let disposal: Promise<void> | undefined;

    const disposeOwnedResources = () => {
      if (!host || disposal) return disposal;
      const ownedHost = host;
      // stopKernel retracts this exact generation synchronously before joining
      // its asynchronous resources.
      disposal = stopKernel(ownedHost).catch((error: unknown) => {
        console.error("[plugin] kernel teardown failed:", error);
      });
      return disposal;
    };

    void (async () => {
      try {
        host = await startKernel(builtinPlugins, controller.signal);
        if (retired) {
          void disposeOwnedResources();
          return;
        }
        setReady(true);
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
