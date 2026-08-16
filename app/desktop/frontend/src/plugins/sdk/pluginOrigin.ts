// Plugin origin registry. The sideload loader records "sideload" for a plugin
// before it registers the artifact with dougong Platform. Anything not
// recorded defaults to "builtin" — that covers the static Host manifest.
// Origin is recorded by the loader, not derived from the name, so a plugin
// cannot spoof it. This leaf stays import-free for sideload registration and
// diagnostics.

export type PluginOrigin = "builtin" | "sideload";

const origins = new Map<string, PluginOrigin>();

export function setPluginOrigin(name: string, origin: PluginOrigin): void {
  origins.set(name, origin);
}

export function pluginOrigin(name: string): PluginOrigin {
  return origins.get(name) ?? "builtin";
}
