// Sideload discovery + loading. Failures are isolated per plugin: a broken
// module logs + is skipped; remaining plugins still load.

import type { LoadResult } from "../sdk/definePlugin";
import type { PluginSpec } from "../sdk/types";
import type { SideloadEntry } from "@/rpc";
import { z } from "zod";
import { RUNTIME_BASE } from "@/main/config";
import { getContainer } from "@/main/container";
import { loadPlugin } from "../sdk/definePlugin";
import { reportPluginError } from "../sdk/errors";
import { pluginOrigin, setPluginOrigin } from "../sdk/pluginOrigin";
import { usePluginStore } from "../sdk/registry";

export { pluginOrigin };

// Sideloaded modules cross the trust boundary — we can't trust their
// default export from TS alone. Validate the shape with Zod before
// handing it to loadPlugin(). The schema is intentionally lenient on
// optional fields (capabilities, requires, contributes…) so older
// plugin bundles still load.
const PluginSpecSchema = z.object({
  name: z.string().min(1),
  version: z.string().min(1),
  setup: z.custom<PluginSpec["setup"]>((v) => typeof v === "function", "setup must be a function"),
  apiVersion: z.string().optional(),
  requires: z.array(z.string()).optional(),
  activationEvents: z.array(z.string()).optional(),
  capabilities: z.array(z.string()).optional(),
  contributes: z.unknown().optional(),
});

/** Discover sideloaded plugins from the Go backend and load each one. */
export async function loadSideloadedPlugins(): Promise<LoadResult[]> {
  // The manifest fetch goes through the container's shell client (the single
  // outbound seam — injectable in tests, ARCHITECTURE §10). The per-plugin
  // module load below is a dynamic import(), inherently glue, so it builds the
  // URL from RUNTIME_BASE directly.
  let infos: SideloadEntry[];
  try {
    infos = await getContainer().shell.sideloadManifest();
  } catch (err) {
    console.warn("[plugin] sideload manifest fetch failed:", err);
    return [];
  }

  const results: LoadResult[] = [];

  for (const info of infos) {
    const url = `${RUNTIME_BASE}${info.url}`;
    let spec: PluginSpec;

    try {
      // Vite warns on dynamic imports of external URLs at build time; the
      // /* @vite-ignore */ comment opts out — these URLs are user-controlled
      // and live behind the Go backend.
      const mod = await import(/* @vite-ignore */ url);
      const def = (mod as { default?: unknown }).default;
      const parsed = PluginSpecSchema.safeParse(def);
      if (!parsed.success) {
        const issues = z.treeifyError(parsed.error);
        const reason = `default export failed PluginSpec schema: ${JSON.stringify(issues)}`;
        reportPluginError(info.id, "setup", new Error(reason));
        results.push({ kind: "skipped", name: info.id, reason });
        continue;
      }
      // The schema is intentionally narrower than PluginSpec (we don't
      // re-validate every nested HostCapability literal etc.) so the
      // assertion below keeps the downstream typing precise.
      spec = parsed.data as PluginSpec;
    } catch (err) {
      console.error(`[plugin] sideload ${info.id} import failed:`, err);
      reportPluginError(info.id, "setup", err);
      results.push({ kind: "failed", name: info.id, error: err });
      continue;
    }

    setPluginOrigin(spec.name, "sideload");
    results.push(await loadPlugin(spec));
  }

  return results;
}

// Sideloaded plugins override their own origin to "sideload" inside
// `loadSideloadedPlugins`.
export function tagAllAsBuiltin(): void {
  for (const name of usePluginStore.getState().loaded.keys()) {
    setPluginOrigin(name, "builtin");
  }
}
