// Read side of the plugin registry — React hooks + imperative lookups
// + lazy-activation helpers. Organized by domain in sibling files;
// this barrel keeps the public API surface identical to the previous
// single-file selectors.ts.

export { setActivator } from "./_helpers";

// Palette / slash / shortcuts (UI command surface)
export {
  lookupCommand,
  lookupShortcut,
  lookupSlashCommand,
  useCommands,
  useShortcuts,
  useSlashCommands,
} from "./commands";

// Composer
export {
  lookupComposerKeyBinding,
  pickComposerPlaceholder,
  useComposerAttachmentSources,
  useComposerModes,
  useComposerStatus,
} from "./composer";

// AG-UI event handler lookups
export { lookupCoreEventHandlers, lookupCustomEventHandlers } from "./events";

// Layout / sidebar / workspace views / settings panes
export {
  useLayoutSlot,
  useSettingsPanes,
  useSidebarRailItems,
  useSidebarSections,
  useWorkspaceViews,
} from "./layout";

// Chat-message surface (content blocks + role + tool)
export {
  lookupToolIcon,
  useContentBlockRenderer,
  useMessageRole,
  useToolActions,
  useToolPreview,
} from "./messages";

// Runtime / data-layer (routes / agents / data providers / RPC hooks /
// plugin error fallback)
export {
  listRoutes,
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupDataProvider,
  pickAgentSource,
  pickPluginErrorFallback,
} from "./runtime";

// Theme + locale
export {
  lookupAccent,
  lookupTheme,
  resolveScheme,
  useAccents,
  useLocales,
  useThemes,
} from "./theme";
