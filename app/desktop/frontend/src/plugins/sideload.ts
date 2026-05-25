// Sideload discovery + loading.
//
// Asks the Go backend for the list of installed plugins, dynamic-imports
// each one's `index.js`, validates the default export, and runs setup
// through the same loadPlugin path the built-ins use.
//
// Failures are isolated per plugin: a broken module logs + is skipped,
// remaining plugins still load.

import type {LoadResult} from "./sdk/definePlugin";
import type { PluginSpec } from "./sdk/types";
import { AGUI_BASE } from "@/lib/http";
import { loadPlugin  } from "./sdk/definePlugin";
import { reportPluginError } from "./sdk/errors";
import { usePluginStore } from "./sdk/registry";

interface SideloadInfo { id: string; url: string }

/**
 * Track where each plugin came from so the Plugins settings pane can mark
 * "built-in" vs "sideloaded". Keyed by spec.name (not by sideload id —
 * spec.name is the canonical identifier used everywhere else).
 */
const pluginOrigins = new Map<string, "builtin" | "sideload">();

export function pluginOrigin(name: string): "builtin" | "sideload" {
  return pluginOrigins.get(name) ?? "builtin";
}

async function fetchSideloadList(): Promise<SideloadInfo[]> {
  const res = await fetch(`${AGUI_BASE}/plugins`);
  if (!res.ok) throw new Error(`GET /plugins → ${res.status}`);
  return (await res.json()) as SideloadInfo[];
}

/**
 * Discover sideloaded plugins from the Go backend and load each one.
 *
 * Returns the load results. The caller (PluginProvider) doesn't need them,
 * but tests do.
 */
export async function loadSideloadedPlugins(): Promise<LoadResult[]> {
  let infos: SideloadInfo[];
  try {
    infos = await fetchSideloadList();
  } catch (err) {
     
    console.warn("[plugin] sideload manifest fetch failed:", err);
    return [];
  }

  const results: LoadResult[] = [];

  for (const info of infos) {
    const url = `${AGUI_BASE}${info.url}`;
    let spec: PluginSpec;

    try {
      // Vite warns on dynamic imports of external URLs at build time; the
      // /* @vite-ignore */ comment opts out — these URLs are user-controlled
      // and live behind the Go backend.
      const mod = await import(/* @vite-ignore */ url);
      const def = (mod as { default?: unknown }).default;
      if (!isPluginSpec(def)) {
        const reason = "default export is not a definePlugin(...) result";
        reportPluginError(info.id, "setup", new Error(reason));
        results.push({ kind: "skipped", name: info.id, reason });
        continue;
      }
      spec = def;
    } catch (err) {
       
      console.error(`[plugin] sideload ${info.id} import failed:`, err);
      reportPluginError(info.id, "setup", err);
      results.push({ kind: "failed", name: info.id, error: err });
      continue;
    }

    pluginOrigins.set(spec.name, "sideload");
    results.push(await loadPlugin(spec));
  }

  return results;
}

function isPluginSpec(v: unknown): v is PluginSpec {
  if (!v || typeof v !== "object") return false;
  const o = v as Record<string, unknown>;
  return (
    typeof o.name === "string" && typeof o.version === "string" && typeof o.setup === "function"
  );
}

// Tag any plugin that's currently loaded as builtin when this module is
// first imported by the host bundle. Sideloaded plugins override their own
// entry to "sideload" inside `loadSideloadedPlugins`.
export function tagAllAsBuiltin(): void {
  for (const name of usePluginStore.getState().loaded.keys()) {
    if (!pluginOrigins.has(name)) pluginOrigins.set(name, "builtin");
  }
}
