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
  const allowedNamespaces = new Set<string>([...allowed, "extensions"]);
  return new Proxy(host, {
    get(target, namespace, receiver) {
      if (
        typeof namespace === "string" &&
        Object.hasOwn(target, namespace) &&
        !allowedNamespaces.has(namespace)
      ) {
        throw new Error(
          `[plugin] ${pluginName}: host.${namespace} is not in this plugin's declared ` +
            `capabilities (add "${namespace}" to spec.capabilities)`,
        );
      }
      return Reflect.get(target, namespace, receiver);
    },
  });
}
