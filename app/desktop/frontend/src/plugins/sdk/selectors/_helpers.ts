// Shared selector helpers — used by every domain-grouped selector file
// in this directory. Not exported from the SDK barrel.

import { useMemo } from "react";
import type { Owned } from "../registryState";

export interface Ordered {
  order?: number;
}

/**
 * Merge two ownership maps by id and sort by order. Used for the three
 * surfaces that have both a "declared placeholder" (rendered until the
 * owning plugin activates) and a "registered real" (the activated
 * component). Registered entries win when ids collide.
 */
export function useDeclaredMerged<D extends { id: string }, R extends { id: string } & Ordered>(
  registered: R[],
  declared: Map<string, Owned<D>>,
  declaredToReal: (d: D, pluginName: string) => R,
): R[] {
  return useMemo(() => {
    const byId = new Map<string, R>();
    for (const o of declared.values()) byId.set(o.value.id, declaredToReal(o.value, o.pluginName));
    for (const r of registered) byId.set(r.id, r);
    return Array.from(byId.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  }, [registered, declared, declaredToReal]);
}

// The real activate-the-plugin impl lives in `definePlugin.ts` (it needs to
// run setup) and installs itself at module-load time via `setActivator`.
// Selectors → definePlugin direction stays clean (no cycle).

type Activator = (pluginName: string) => Promise<void>;
let pluginActivator: Activator | null = null;

export function setActivator(fn: Activator): void {
  pluginActivator = fn;
}

/**
 * Trigger the configured activator for `pluginName`. Used by lazy
 * placeholder components (workspace views / settings panes) whose only
 * job is to nudge the plugin into running setup; once setup completes,
 * the selector hooks re-emit a list where the real component replaces
 * the placeholder.
 */
export async function runActivator(pluginName: string): Promise<void> {
  if (!pluginActivator) {
    console.error(`[plugin] activator not wired; cannot lazily activate ${pluginName}`);
    return;
  }
  await pluginActivator(pluginName);
}
