// Read side of the plugin registry. Plain reads (a list / one item by key) use
// the generic substrate below; this barrel only adds the selectors with real
// logic on top of it (declared-merge, weighted-random, priority pick, cached
// sub-index, owner attribution).

export { setActivator } from "./_helpers";

// Open extension points — the one read API for plain reads (kernel + plugins).
export {
  lookupExtensionByKey,
  lookupExtensionPoint,
  useExtensionByKey,
  useExtensionPoint,
} from "./extensions";

// Palette commands (registered + declared merge) + slash-command pairing +
// owner attribution.
export {
  executeCommand,
  lookupCommandOwner,
  lookupSlashCommandOwner,
  useCommands,
  useSlashCommands,
} from "./commands";

// Composer placeholder weighted-random pick.
export { pickComposerPlaceholder } from "./composer";

// StreamEvent handler fan-out (cached sub-index, hit per event).
export { lookupStreamHandlers, lookupCustomHandlers } from "./events";

// Layout slot (sub-keyed by slot) + workspace views / settings panes
// (registered + declared merge).
export {
  useContextDockDestinations,
  useLayoutSlot,
  useSettingsPanes,
  useWorkIndexItems,
  useWorkspaceViews,
} from "./layout";

// Tool owner attribution + per-message citation sources.
export { lookupToolActionOwner, lookupToolViewOpenerOwner, useCitationSources } from "./messages";

// Runtime / data-layer: priority picks + data-provider fetcher.
export {
  lookupDataProvider,
  pickAgentSource,
  pickPluginErrorFallback,
  resolveAgentRunStartOptions,
} from "./runtime";

// Theme scheme resolution.
export { resolveScheme } from "./theme";
