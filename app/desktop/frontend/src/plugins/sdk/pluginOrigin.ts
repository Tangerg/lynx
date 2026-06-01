// Plugin origin registry. The sideload loader records "sideload" for a plugin
// *before* it calls `loadPlugin`, so the load path can tell a third-party
// bundle from a first-party built-in and enforce the right trust policy
// (sideload default-deny). Anything not recorded defaults to "builtin"
// (trusted) — that covers the static manifest. Origin is recorded by the
// loader, not derived from the name, so a plugin can't spoof it.
//
// A leaf module (no imports) so both the loader (`sideload.ts`) and the load
// path (`definePlugin.ts`) can use it without a dependency cycle.

export type PluginOrigin = "builtin" | "sideload";

const origins = new Map<string, PluginOrigin>();

export function setPluginOrigin(name: string, origin: PluginOrigin): void {
  origins.set(name, origin);
}

export function pluginOrigin(name: string): PluginOrigin {
  return origins.get(name) ?? "builtin";
}
