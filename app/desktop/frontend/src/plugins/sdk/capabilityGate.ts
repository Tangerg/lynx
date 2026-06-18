// Capability gating for a bound Host: wrap it so any namespace the plugin
// didn't declare in `capabilities` throws on access. Lives apart from host.ts
// (the Host construction) because this is a self-contained policy layer —
// host.ts builds the full Host, this restricts it.

import type { Host, HostCapability } from "./types";

/**
 * Wrap a host such that any access to a namespace the plugin didn't declare in
 * `capabilities` throws with a clear error message. `extensions` is always
 * reachable — it's the universal write path, gated per-point inside
 * `contribute` (by the point's `capability`), not at the namespace level.
 */
export function restrictHost(host: Host, pluginName: string, allowed: HostCapability[]): Host {
  const allowedSet = new Set<HostCapability>([...allowed, "extensions"]);
  const denied: Record<string, unknown> = {};
  for (const key of Object.keys(host) as Array<keyof Host>) {
    if (allowedSet.has(key as HostCapability)) {
      denied[key as string] = host[key];
    } else {
      denied[key as string] = createDenyProxy(pluginName, key as string);
    }
  }
  return denied as unknown as Host;
}

function createDenyProxy(pluginName: string, namespace: string): unknown {
  const explain = (prop: string) =>
    new Error(
      `[plugin] ${pluginName}: host.${namespace}${prop ? `.${prop}` : ""} ` +
        `is not in this plugin's declared capabilities (add "${namespace}" to spec.capabilities)`,
    );
  // Trap both function-style (host.notify(...)) and property access.
  const denied = function denied() {
    throw explain("");
  };
  return new Proxy(denied, {
    get(_, prop) {
      throw explain(String(prop));
    },
    apply() {
      throw explain("");
    },
  });
}
