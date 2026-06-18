// Point ids that the registry store fires from *inside* its own actions
// (markAppReady / registerLoaded / unload). Those actions can't import
// `kernelPoints` — that module imports the registry, so the dependency only
// runs one way — yet they need to know which `extensions` entries are the
// lifecycle hooks to fan out. This zero-dependency module is the single source
// of truth: `kernelPoints` builds the typed handles from these strings and the
// store filters `extensions` by them.

export const LIFECYCLE_POINT_IDS = {
  ready: "lyra.lifecycle.ready",
  beforeUnload: "lyra.lifecycle.beforeUnload",
  pluginLoad: "lyra.plugins.onLoad",
  pluginUnload: "lyra.plugins.onUnload",
} as const;
