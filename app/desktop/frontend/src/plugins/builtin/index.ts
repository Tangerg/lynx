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
import appearance from "./settings/appearance";
import personalization from "./settings/personalization";
import chatSearch from "./chat/chat-search";
import commandPalette from "./command/command-palette";
import {
  composerChips,
  composerKeymap,
  composerModes,
  composerPlaceholders,
  composerSend,
  composerToolbar,
} from "./chat/composer";
import connectionSettings from "./settings/connection-settings";
import previewBlocks from "./chat/preview-blocks";
import bootstrap from "./agent/bootstrap";
import conversationExport from "./workspace/conversation-export";
import coreReducer from "./agent/core-reducer";
import {
  defaultAccents,
  defaultCommands,
  defaultConfig,
  defaultData,
  defaultRoles,
  defaultTitle,
} from "./defaults";
import diagnostics from "./workspace/diagnostics";
import globalKeymap from "./command/global-keymap";
import iconGallery from "./settings/icon-gallery";
import rpcAgent from "./agent/rpc-agent";
import { kernelChat, kernelSettings, kernelSidebar } from "./shell/kernel";
import { localesPack } from "./i18n";
import mainRoute from "./shell/main-route";
import { messageCopy, messageEdit, messageRegenerate } from "./chat/message-actions";
import planProgress from "./chat/plan-progress";
import pluginsPane from "./settings/plugins-pane";
import shortcuts from "./command/shortcuts";
import {
  sidebarFooter,
  sidebarProjects,
  sidebarRailActions,
  sidebarRailBottom,
  sidebarRailSessions,
  sidebarSearch,
  sidebarSessions,
} from "./sidebar";
import slashHints from "./chat/slash-hints";
import { statusNotifications, statusPill } from "./shell/status";
import { tasksPill } from "./workspace/tasks";
import { themesPack } from "./theme";
import toaster from "./shell/toaster";
import { toolActions, toolIcons } from "./chat/tools/meta";
import { bash, diff, file, grep } from "./chat/tools/previews";
import topbarNewTab from "./shell/topbar-new-tab";
import welcomeScreen from "./shell/welcome-screen";
import {
  agentDocsView,
  diffView,
  filesView,
  notificationsView,
  planView,
  runSummaryView,
  skillsView,
  terminalView,
  timelineView,
  toolsView,
} from "./workspace/workspace-views";

// ---------------------------------------------------------------------------
// Protocol — fold v2 RunEvents (run.* / item.* / state.*) into view state.
// All semantics (messages, reasoning, tools, plan, questions, HITL) are
// first-class Items now, so the single core-reducer owns the whole fold;
// `custom` StreamEvents are reserved for third-party plugins.
// ---------------------------------------------------------------------------
const protocol: PluginSpec[] = [coreReducer];

// ---------------------------------------------------------------------------
// Configuration & infrastructure.
// ---------------------------------------------------------------------------
const infrastructure: PluginSpec[] = [
  defaultConfig,
  // bootstrap after defaultConfig so api.localToken is set before the
  // handshake builds the RpcClient (API.md §2 Lifecycle).
  bootstrap,
  defaultData,
  rpcAgent,
  defaultTitle,
  defaultAccents,
  themesPack,
  localesPack,
  mainRoute,
];

// ---------------------------------------------------------------------------
// Message rendering — roles, content blocks, per-message decorations.
// ---------------------------------------------------------------------------
// Built-in content blocks (text / tool / reasoning / plan / approval /
// question) render directly in the message module — no plugin here. This
// group is roles + per-message actions + the extension-only preview blocks.
const messageRendering: PluginSpec[] = [
  defaultRoles,
  messageCopy,
  messageEdit,
  messageRegenerate,
  previewBlocks,
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
  composerChips,
  composerModes,
  composerToolbar,
  composerPlaceholders,
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
  skillsView,
  agentDocsView,
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
