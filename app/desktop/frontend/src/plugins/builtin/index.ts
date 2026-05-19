// Manifest of all built-in plugins.
//
// PluginProvider loads them in declared order at app startup. Order
// matters in two situations:
//   1. Last-write-wins slots (preview, theme, etc.) — later plugins
//      override earlier ones.
//   2. Setup-time snapshots (defaultCommands snapshots the accent
//      registry) — the snapshotting plugin must load after its source.
//
// The groups below collect plugins by concern. Within a group, ordering
// is informational; between groups, the comment on the group records the
// "must load before X" constraint when there is one.

import appearance from "./appearance";
import approvalBlock from "./approval-block";
import approvalHandler from "./approval-handler";
import bash from "./bash";
import checkpointBlock from "./checkpoint-block";
import codeBlock from "./code-block";
import codeProposalHandler from "./code-proposal-handler";
import commandPalette from "./command-palette";
import composerChips from "./composer-chips";
import composerHint from "./composer-hint";
import composerKeymap from "./composer-keymap";
import composerModes from "./composer-modes";
import composerPlaceholders from "./composer-placeholders";
import composerSend from "./composer-send";
import composerToolbar from "./composer-toolbar";
import coreReducer from "./core-reducer";
import defaultCommands from "./default-commands";
import defaultConfig from "./default-config";
import defaultData from "./default-data";
import defaultRoles from "./default-roles";
import defaultThemes from "./default-themes";
import defaultTitle from "./default-title";
import demo from "./demo";
import diff from "./diff";
import file from "./file";
import grep from "./grep";
import httpAgent from "./http-agent";
import inspectorDiff from "./inspector-diff";
import inspectorFiles from "./inspector-files";
import inspectorNotifications from "./inspector-notifications";
import inspectorPlan from "./inspector-plan";
import inspectorTerminal from "./inspector-terminal";
import inspectorTools from "./inspector-tools";
import mainRoute from "./main-route";
import messageCopy from "./message-copy";
import planBlock from "./plan-block";
import planHandler from "./plan-handler";
import pluginsPane from "./plugins-pane";
import reasoningBlock from "./reasoning-block";
import sampleAttachments from "./sample-attachments";
import searchBlock from "./search-block";
import searchResultsHandler from "./search-results-handler";
import shellChat from "./shell-chat";
import shellInspector from "./shell-inspector";
import shellSettings from "./shell-settings";
import shellSidebar from "./shell-sidebar";
import shortcuts from "./shortcuts";
import sidebarBrand from "./sidebar-brand";
import sidebarFooter from "./sidebar-footer";
import sidebarProjects from "./sidebar-projects";
import sidebarRailActions from "./sidebar-rail-actions";
import sidebarRailBottom from "./sidebar-rail-bottom";
import sidebarRailSessions from "./sidebar-rail-sessions";
import sidebarSearch from "./sidebar-search";
import sidebarSessions from "./sidebar-sessions";
import slashHints from "./slash-hints";
import statusPill from "./status-pill";
import telemetryHandler from "./telemetry-handler";
import toaster from "./toaster";
import toolActions from "./tool-actions";
import toolIcons from "./tool-icons";
import topbarNewTab from "./topbar-new-tab";
import welcomeScreen from "./welcome-screen";
import type { PluginSpec } from "../sdk";

// ---------------------------------------------------------------------------
// Protocol — fold AG-UI events into view state.
// ---------------------------------------------------------------------------
const protocol: PluginSpec[] = [
  coreReducer,        // RUN_* / TEXT_* / TOOL_* / REASONING_*
  planHandler,        // CUSTOM "lyra.plan" / "lyra.plan-block"
  codeProposalHandler,
  searchResultsHandler,
  approvalHandler,
  telemetryHandler,
];

// ---------------------------------------------------------------------------
// Configuration & infrastructure.
// `defaultConfig` must precede `httpAgent` (httpAgent reads api.baseUrl).
// `defaultData` must precede any UI that triggers a query.
// ---------------------------------------------------------------------------
const infrastructure: PluginSpec[] = [
  defaultConfig,
  defaultData,
  httpAgent,
  defaultTitle,
  defaultThemes,
  mainRoute,
];

// ---------------------------------------------------------------------------
// Message rendering — roles, content blocks, per-message decorations.
// ---------------------------------------------------------------------------
const messageRendering: PluginSpec[] = [
  defaultRoles,
  messageCopy,
  planBlock,
  codeBlock,
  searchBlock,
  approvalBlock,
  checkpointBlock,
  reasoningBlock,
];

// ---------------------------------------------------------------------------
// Tool rendering — previews, header actions, icon glyph map.
// ---------------------------------------------------------------------------
const toolRendering: PluginSpec[] = [
  bash, diff, file, grep,
  toolActions,
  toolIcons,
];

// ---------------------------------------------------------------------------
// Composer — slash commands, modes, toolbar, status chips, send & hint.
// ---------------------------------------------------------------------------
const composer: PluginSpec[] = [
  slashHints, demo,
  composerChips, composerModes, composerToolbar,
  composerPlaceholders, sampleAttachments,
  composerKeymap, composerHint, composerSend,
];

// ---------------------------------------------------------------------------
// Settings & inspector panes (each spec is independent).
// ---------------------------------------------------------------------------
const panes: PluginSpec[] = [
  appearance, pluginsPane,
  inspectorDiff, inspectorTerminal, inspectorFiles,
  inspectorPlan, inspectorTools, inspectorNotifications,
];

// ---------------------------------------------------------------------------
// Shell layout regions — fill the named slots in AgentClientPage.
// ---------------------------------------------------------------------------
const shell: PluginSpec[] = [
  shellSidebar, shellChat, shellInspector, shellSettings,
];

// ---------------------------------------------------------------------------
// Sidebar internals — sections in the expanded view, items in the rail.
// ---------------------------------------------------------------------------
const sidebar: PluginSpec[] = [
  sidebarBrand, sidebarSearch,
  sidebarProjects, sidebarSessions, sidebarFooter,
  sidebarRailActions, sidebarRailSessions, sidebarRailBottom,
];

// ---------------------------------------------------------------------------
// Overlays — toasts, command palette, status pill, welcome screen, default
// commands (defaultCommands snapshots accents at setup time, so it must
// follow `defaultThemes` from the infrastructure group).
// ---------------------------------------------------------------------------
const overlays: PluginSpec[] = [
  toaster, commandPalette, defaultCommands,
  statusPill, welcomeScreen, topbarNewTab,
  shortcuts,
];

export const builtinPlugins: PluginSpec[] = [
  ...protocol,
  ...infrastructure,
  ...messageRendering,
  ...toolRendering,
  ...composer,
  ...panes,
  ...shell,
  ...sidebar,
  ...overlays,
];
