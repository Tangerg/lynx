// Read side of the plugin registry. Plain reads (a list / one item by key) use
// the generic substrate below; this barrel only adds the selectors with real
// logic on top of it (weighted-random, priority pick, cached sub-index, owner
// attribution).

// Open extension points — the one read API for plain reads (kernel + plugins).
export {
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
  useExtensionByKey,
  useExtensionPoint,
} from "./extensions";

// Command execution + slash-command pairing and owner attribution.
export { executeCommand, lookupSlashCommandOwner, useSlashCommands } from "./commands";

// Composer placeholder weighted-random pick.

// StreamEvent handler fan-out (cached sub-index, hit per event).
export { lookupStreamHandlers } from "./events";

// Layout slot (sub-keyed by slot) + workspace views / settings panes.
export {
  useContextDockDestinations,
  useLayoutSlot,
  useSettingsPanes,
  useWorkIndexItems,
  useWorkspaceViews,
} from "./layout";

// Tool owner attribution.
export { lookupToolActionOwner, lookupToolViewOpenerOwner } from "./messages";

// Runtime / data-layer: priority picks + data-provider fetcher.
export {
  lookupDataProvider,
  pickAgentSource,
  pickPluginErrorFallback,
  resolveAgentRunStartOptions,
} from "./runtime";

// Theme scheme resolution.
