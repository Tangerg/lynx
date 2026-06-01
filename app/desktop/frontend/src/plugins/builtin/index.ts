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
} from "./chat/agui-handlers";
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
import {
  approvalBlock,
  checkpointBlock,
  codeBlock,
  planBlock,
  questionBlock,
  reasoningBlock,
  searchBlock,
} from "./chat/content-blocks";
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
} from "./shell/defaults";
import demo from "./samples/demo";
import diagnostics from "./workspace/diagnostics";
import globalKeymap from "./command/global-keymap";
import httpAgent from "./agent/http-agent";
import iconGallery from "./settings/icon-gallery";
import rpcAgent from "./agent/rpc-agent";
import { kernelChat, kernelSettings, kernelSidebar } from "./shell/kernel";
import { localesPack } from "./i18n";
import mainRoute from "./shell/main-route";
import { messageCopy, messageEdit, messageRegenerate } from "./chat/message-actions";
import planProgress from "./chat/plan-progress";
import pluginsPane from "./settings/plugins-pane";
import sampleAttachments from "./samples/sample-attachments";
import shortcuts from "./command/shortcuts";
import {
  sidebarFooter,
  sidebarProjects,
  sidebarRailActions,
  sidebarRailBottom,
  sidebarRailSessions,
  sidebarSearch,
  sidebarSessions,
} from "./shell/sidebar";
import slashHints from "./chat/slash-hints";
import { statusNotifications, statusPill } from "./shell/status";
import { tasksPill } from "./workspace/tasks";
import { themesPack } from "./theme";
import toaster from "./shell/toaster";
import { toolActions, toolIcons } from "./chat/tool-meta";
import { bash, diff, file, grep } from "./chat/tool-previews";
import topbarNewTab from "./shell/topbar-new-tab";
import welcomeScreen from "./shell/welcome-screen";
import {
  diffView,
  filesView,
  notificationsView,
  planView,
  runSummaryView,
  terminalView,
  timelineView,
  toolsView,
} from "./workspace/workspace-views";

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
  themesPack,
  localesPack,
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
