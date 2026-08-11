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
import approvalsPane from "./settings/approvals";
import personalization from "./settings/personalization";
import chatSearch from "./chat/chat-search";
import {
  composerBootstrap,
  composerKeymap,
  composerRunOptions,
  composerSend,
  composerToolbar,
} from "./chat/composer";
import connectionSettings from "./settings/connection-settings";
import previewBlocks from "./chat/preview-blocks";
import agentBootstrap from "./agent/bootstrap";
import observability from "./observability";
import runtime from "./runtime";
import conversationExport from "./workspace/conversation-export";
import contextDockDestinations from "./workspace/context-dock";
import agentFold from "./agent/public/foldPlugin";
import {
  defaultAccents,
  defaultCommands,
  defaultDataProviders,
  defaultRoles,
  defaultTitle,
} from "./defaults";
import diagnostics from "./workspace/diagnostics";
import workspaceBootstrap from "./workspace/bootstrap";
import workspaceEvents from "./workspace/events";
import workspaceSessionNavigation from "./workspace/session-navigation";
import globalKeymap from "./command/global-keymap";
import sessionSearch from "./command/session-search";
import hooksPane from "./settings/hooks";
import schedulesPane from "./settings/schedules";
import iconGallery from "./settings/icon-gallery";
import mcpServersPane from "./settings/mcp-servers";
import rpcAgent from "./agent/rpc-agent";
import { kernelChat, kernelSettings, kernelSidebar } from "./shell/kernel";
import nativeShell from "./shell/native-shell";
import providerSetup from "./shell/provider-setup";
import { localesPack } from "./i18n";
import mainRoute from "./shell/main-route";
import {
  messageCopy,
  messageEdit,
  messageFeedback,
  messageRegenerate,
} from "./chat/message-actions";
import goal from "./chat/goal";
import narrativeRails from "./chat/narrative-rails";
import planProgress from "./chat/plan-progress";
import recipesSlash from "./chat/recipes";
import pluginsPane from "./settings/plugins-pane";
import providersPane from "./settings/providers";
import sessionUsage from "./chat/session-usage";
import shortcuts from "./command/shortcuts";
import usagePane from "./settings/usage";
import { sidebarActions, sidebarFooter, sidebarProjects, sidebarRecents } from "./sidebar";
import slashHints from "./chat/slash-hints";
import { completionNotify, statusNotifications, windowTitle } from "./shell/status";
import { tasksPill } from "./workspace/tasks";
import { appearancePack } from "./theme";
import toaster from "./shell/toaster";
import { toolActions, toolIcons } from "./chat/tools/meta";
import toolViewOpener from "./workspace/tool-view-opener";
import {
  askUserPreview,
  shellPreview,
  diff,
  file,
  globPreview,
  grep,
  httpPreviews,
  lspPreviews,
  recallPreviews,
  schedulePreview,
  skillPreview,
  taskPreview,
  toolSearchPreviewPlugin,
  webSearchPreview,
} from "./chat/tools/previews";
import {
  agentDocsView,
  diffView,
  fileView,
  filesView,
  fileTreeView,
  knowledgeView,
  agentMemoryView,
  inboxView,
  notificationsView,
  planView,
  recipesView,
  codebaseView,
  runSummaryView,
  searchView,
  skillsView,
  skillLibraryView,
  skillProposalsView,
  terminalView,
  timelineView,
  toolStatsView,
  toolsView,
} from "./workspace/workspace-views";

// Agent fold — fold v2 RunEvents (run.* / item.* / state.*) into view state.
// All semantics (messages, reasoning, tools, plan, questions, HITL) are
// first-class Items now, so the built-in agent fold owns the whole fold;
// `custom` StreamEvents are reserved for third-party plugins.
const protocol: PluginSpec[] = [agentFold];

// Configuration & infrastructure.

const infrastructure: PluginSpec[] = [
  nativeShell,
  observability,
  agentBootstrap,
  runtime,
  workspaceBootstrap,
  defaultDataProviders,
  // After bootstrap: watches the discovery result and opens the app's one
  // runtime.subscribe stream.
  workspaceEvents,
  workspaceSessionNavigation,
  rpcAgent,
  defaultTitle,
  defaultAccents,
  appearancePack,
  localesPack,
  mainRoute,
];

// Message rendering — roles, content blocks, per-message decorations.

// Built-in content blocks (text / tool / reasoning / plan / approval /
// question) render directly in the message module — no plugin here. This
// group is roles + per-message actions + the extension-only preview blocks.
const messageRendering: PluginSpec[] = [
  defaultRoles,
  messageCopy,
  messageEdit,
  messageRegenerate,
  messageFeedback,
  previewBlocks,
];

// Tool rendering — previews, header actions, icon glyph map.
//
// Exported because the visual fixture renders transcripts and used to hand-pick four
// of these, so the components for the rest — including the diff and grep previews the
// canonical snapshots carry results for — drew raw JSON in every fixture while
// production drew the real thing. A list to be kept in sync by hand is a list that
// drifts silently.
export const toolRenderingPlugins: PluginSpec[] = [
  shellPreview,
  diff,
  file,
  grep,
  globPreview,
  lspPreviews,
  skillPreview,
  taskPreview,
  askUserPreview,
  webSearchPreview,
  recallPreviews,
  toolSearchPreviewPlugin,
  schedulePreview,
  httpPreviews,
  toolActions,
  toolViewOpener,
  toolIcons,
];

// Composer — slash commands, modes, toolbar, status chips, send & hint.

const composer: PluginSpec[] = [
  composerBootstrap,
  slashHints,
  // After slashHints so a user recipe named like a built-in hint wins the
  // shared slash key (it carries a real run handler; the hint is display-only).
  recipesSlash,
  composerToolbar,
  composerRunOptions,
  composerKeymap,
  composerSend,
];

// Settings panes + workspace views (each spec is independent).

const panes: PluginSpec[] = [
  appearance,
  approvalsPane,
  personalization,
  connectionSettings,
  pluginsPane,
  providersPane,
  usagePane,
  mcpServersPane,
  hooksPane,
  schedulesPane,
  contextDockDestinations,
  diffView,
  fileView,
  terminalView,
  filesView,
  fileTreeView,
  planView,
  timelineView,
  toolStatsView,
  runSummaryView,
  toolsView,
  skillsView,
  skillLibraryView,
  skillProposalsView,
  recipesView,
  codebaseView,
  searchView,
  agentDocsView,
  knowledgeView,
  agentMemoryView,
  inboxView,
  notificationsView,
  diagnostics,
];

// Kernel layout regions — fill the named slots in AgentClientPage.

const kernel: PluginSpec[] = [kernelSidebar, kernelChat, kernelSettings];

// Sidebar internals — the sections that fill the expanded work-index view.

const sidebar: PluginSpec[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

// Overlays + chrome — toasts, command palette, status bar, …

const overlays: PluginSpec[] = [
  toaster,
  chatSearch,
  defaultCommands,
  tasksPill,
  statusNotifications,
  completionNotify,
  windowTitle,
  shortcuts,
  globalKeymap,
  sessionSearch,
  iconGallery,
  narrativeRails,
  goal,
  planProgress,
  providerSetup,
  sessionUsage,
  conversationExport,
];

export const builtinPlugins: PluginSpec[] = [
  ...protocol,
  ...infrastructure,
  ...messageRendering,
  ...toolRenderingPlugins,
  ...composer,
  ...panes,
  ...kernel,
  ...sidebar,
  ...overlays,
];
