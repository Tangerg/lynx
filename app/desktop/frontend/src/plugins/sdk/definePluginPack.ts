// `definePluginPack` — bundle N child plugins behind one manifest entry.
//
// Orthogonal to the extension-point substrate: a pack is just a regular plugin
// whose setup orchestrates its children. It loads each child in array order via
// the existing `host.plugins.load` (so each child gets its own bound host,
// capabilities, disposables, and error isolation), then runs the pack's own
// `setup` — which can consume whatever points the children filled. Unload
// cascades in reverse (pack cleanup first, then children newest-first).
//
// Trust: children inherit the pack's origin, so a sideload pack's children are
// also default-deny (no privilege escalation). Loading children needs the
// `plugins` capability — a built-in pack has it implicitly; a sideload pack
// must declare it (dangerous).

import type { PluginPackSpec, PluginSpec } from "./types";
import { definePlugin } from "./definePlugin";
import { pluginOrigin, setPluginOrigin } from "./pluginOrigin";

export function definePluginPack(pack: PluginPackSpec): PluginSpec {
  return definePlugin({
    name: pack.name,
    version: pack.version,
    apiVersion: pack.apiVersion,
    requires: pack.requires,
    capabilities: pack.capabilities,
    setup: async (ctx) => {
      const origin = pluginOrigin(pack.name);
      const loaded: string[] = [];
      for (const child of pack.children) {
        setPluginOrigin(child.name, origin);
        await ctx.host.plugins.load(child);
        loaded.push(child.name);
      }
      const cleanup = await pack.setup?.(ctx);
      return () => {
        if (typeof cleanup === "function") cleanup();
        for (const name of loaded.reverse()) ctx.host.plugins.unload(name);
      };
    },
  });
}
