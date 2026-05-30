// Manifest of all built-in plugins.
//
// `loadPlugins` performs a topological sort over `spec.requires`. This
// array's order is only a tie-breaker between independent plugins — the
// groups below are for *human* readability, not load-order semantics.
// Any "must load before X" relationship lives inside the dependent
// plugin's `requires: [...]` field.
//
// Slot ordering (which contribution wins for last-write-wins slots like
// previews / themes) is still array-order driven, so keep destructive
// overrides later in the manifest.

import type { PluginSpec } from "../sdk";
import {
  approvalHandler,
  codeProposalHandler,
  planHandler,
  questionHandler,
  searchResultsHandler,
  telemetryHandler,
} from "./agui-handlers";
import appearance from "./appearance";
import personalization from "./personalization";
import chatSearch from "./chat-search";
import commandPalette from "./command-palette";
import {
  composerChips,
  composerKeymap,
  composerModes,
  composerPlaceholders,
  composerSend,
  composerToolbar,
} from "./composer";
import connectionSettings from "./connection-settings";
import {
  approvalBlock,
  checkpointBlock,
  codeBlock,
  planBlock,
  questionBlock,
  reasoningBlock,
  searchBlock,
} from "./content-blocks";
import bootstrap from "./bootstrap";
import conversationExport from "./conversation-export";
import coreReducer from "./core-reducer";
import customTheme from "./custom-theme";
import {
  defaultAccents,
  defaultCommands,
  defaultConfig,
  defaultData,
  defaultRoles,
  defaultTitle,
} from "./defaults";
import demo from "./demo";
import diagnostics from "./diagnostics";
import globalKeymap from "./global-keymap";
import httpAgent from "./http-agent";
import iconGallery from "./icon-gallery";
import rpcAgent from "./rpc-agent";
import { kernelChat, kernelSettings, kernelSidebar } from "./kernel";
import {
  localeDe,
  localeEn,
  localeEs,
  localeFr,
  localeJa,
  localeKo,
  localeZh,
  localeZhTW,
} from "./locales";
import mainRoute from "./main-route";
import { messageCopy, messageEdit, messageRegenerate } from "./message-actions";
import planProgress from "./plan-progress";
import pluginsPane from "./plugins-pane";
import sampleAttachments from "./sample-attachments";
import shortcuts from "./shortcuts";
import {
  sidebarFooter,
  sidebarProjects,
  sidebarRailActions,
  sidebarRailBottom,
  sidebarRailSessions,
  sidebarSearch,
  sidebarSessions,
} from "./sidebar";
import slashHints from "./slash-hints";
import { statusNotifications, statusPill } from "./status";
import { tasksPill } from "./tasks";
import { builtinThemes } from "./themes";
import toaster from "./toaster";
import { toolActions, toolIcons } from "./tool-meta";
import { bash, diff, file, grep } from "./tool-previews";
import topbarNewTab from "./topbar-new-tab";
import welcomeScreen from "./welcome-screen";
import {
  diffView,
  filesView,
  notificationsView,
  planView,
  runSummaryView,
  terminalView,
  timelineView,
  toolsView,
} from "./workspace-views";

// ---------------------------------------------------------------------------
// Protocol — fold AG-UI events into view state.
// ---------------------------------------------------------------------------
const protocol: PluginSpec[] = [
  coreReducer, // RUN_* / TEXT_* / TOOL_* / REASONING_*
  planHandler, // CUSTOM "lyra.plan" / "lyra.plan-block"
  codeProposalHandler,
  searchResultsHandler,
  approvalHandler,
  questionHandler, // CUSTOM "lyra.question" / "lyra.question-result"
  telemetryHandler,
];

// ---------------------------------------------------------------------------
// Configuration & infrastructure.
// ---------------------------------------------------------------------------
const infrastructure: PluginSpec[] = [
  defaultConfig,
  // bootstrap after defaultConfig so api.localToken is set before the
  // handshake builds the RpcClient (INTEGRATION.md §2).
  bootstrap,
  defaultData,
  httpAgent,
  rpcAgent,
  defaultTitle,
  defaultAccents,
  ...builtinThemes,
  customTheme,
  // Locale packs — each language is its own plugin so a third-party
  // can ship Vietnamese / Arabic / ... without touching the kernel.
  // English isn't strictly needed here (its bundle is bootstrapped by
  // `lib/i18n.ts`) but the plugin registers the picker entry.
  localeEn,
  localeZh,
  localeZhTW,
  localeJa,
  localeKo,
  localeEs,
  localeFr,
  localeDe,
  mainRoute,
];

// ---------------------------------------------------------------------------
// Message rendering — roles, content blocks, per-message decorations.
// ---------------------------------------------------------------------------
const messageRendering: PluginSpec[] = [
  defaultRoles,
  messageCopy,
  messageEdit,
  messageRegenerate,
  planBlock,
  codeBlock,
  searchBlock,
  approvalBlock,
  questionBlock,
  checkpointBlock,
  reasoningBlock,
];

// ---------------------------------------------------------------------------
// Tool rendering — previews, header actions, icon glyph map.
// ---------------------------------------------------------------------------
const toolRendering: PluginSpec[] = [bash, diff, file, grep, toolActions, toolIcons];

// ---------------------------------------------------------------------------
// Composer — slash commands, modes, toolbar, status chips, send & hint.
// ---------------------------------------------------------------------------
const composer: PluginSpec[] = [
  slashHints,
  demo,
  composerChips,
  composerModes,
  composerToolbar,
  composerPlaceholders,
  sampleAttachments,
  composerKeymap,
  composerSend,
];

// ---------------------------------------------------------------------------
// Settings panes + workspace views (each spec is independent).
// ---------------------------------------------------------------------------
const panes: PluginSpec[] = [
  appearance,
  personalization,
  connectionSettings,
  pluginsPane,
  diffView,
  terminalView,
  filesView,
  planView,
  timelineView,
  runSummaryView,
  toolsView,
  notificationsView,
  diagnostics,
];

// ---------------------------------------------------------------------------
// Kernel layout regions — fill the named slots in AgentClientPage.
// ---------------------------------------------------------------------------
const kernel: PluginSpec[] = [kernelSidebar, kernelChat, kernelSettings];

// ---------------------------------------------------------------------------
// Sidebar internals — sections in the expanded view, items in the rail.
// ---------------------------------------------------------------------------
const sidebar: PluginSpec[] = [
  sidebarSearch,
  sidebarProjects,
  sidebarSessions,
  sidebarFooter,
  sidebarRailActions,
  sidebarRailSessions,
  sidebarRailBottom,
];

// ---------------------------------------------------------------------------
// Overlays — toasts, command palette, status pill, welcome screen, …
// ---------------------------------------------------------------------------
const overlays: PluginSpec[] = [
  toaster,
  commandPalette,
  chatSearch,
  defaultCommands,
  statusPill,
  tasksPill,
  statusNotifications,
  welcomeScreen,
  topbarNewTab,
  shortcuts,
  globalKeymap,
  iconGallery,
  planProgress,
  conversationExport,
];

export const builtinPlugins: PluginSpec[] = [
  ...protocol,
  ...infrastructure,
  ...messageRendering,
  ...toolRendering,
  ...composer,
  ...panes,
  ...kernel,
  ...sidebar,
  ...overlays,
];
