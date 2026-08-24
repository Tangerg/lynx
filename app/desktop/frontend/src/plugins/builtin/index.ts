// Manifest of all built-in plugins.
//
// dougong starts this set as one Host transaction and resolves declared
// service contracts from each plugin's `requires` / `provides`. This array's
// order is only a tie-breaker between independent plugins — the groups below
// are for human readability, not dependency semantics.
//
// Slot ordering (which contribution wins for last-write-wins slots like
// previews / themes) is still array-order driven, so keep destructive
// overrides later in the manifest.

import type { AnyPlugin } from "dougong";
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
import agentBootstrap from "./agent/bootstrap";
import observability from "./observability";
import runtime from "./runtime";
import conversationExport from "./workspace/conversation-export";
import contextDockDestinations from "./workspace/context-dock";
import agentFold from "./agent/bootstrap/foldPlugin";
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
import { localePlugins } from "./i18n";
import mainRoute from "./shell/main-route";
import navigationBootstrap from "./navigation/bootstrap";
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
import contextUsage from "./chat/context-usage";
import shortcuts from "./command/shortcuts";
import usagePane from "./settings/usage";
import { sidebarActions, sidebarFooter, sidebarProjects, sidebarRecents } from "./sidebar";
import slashHints from "./chat/slash-hints";
import { completionNotify, statusNotifications, windowTitle } from "./shell/status";
import { tasksPill } from "./workspace/tasks";
import { appearancePlugins } from "./theme";
import toaster from "./shell/toaster";
import { toolActions, toolIcons } from "./chat/tools/meta";
import toolViewOpener from "./workspace/tool-view-opener";
import {
  askUserPreview,
  shellPreview,
  applyPatchPreview,
  file,
  globPreview,
  goalPreviews,
  grep,
  httpPreviews,
  lspPreviews,
  planPreviews,
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
// first-class Items now, so the built-in agent fold owns the whole fold.
const protocol: AnyPlugin[] = [agentFold];

// Configuration & infrastructure.

const infrastructure: AnyPlugin[] = [
  nativeShell,
  observability,
  navigationBootstrap,
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
  ...appearancePlugins,
  ...localePlugins,
  mainRoute,
];

// Message rendering — roles and per-message actions. Protocol content blocks
// render directly in the message module; there is no second renderer registry.
const messageRendering: AnyPlugin[] = [
  defaultRoles,
  messageCopy,
  messageEdit,
  messageRegenerate,
  messageFeedback,
];

// Tool rendering — previews, header actions, icon glyph map.
//
// Exported so the visual fixture installs the same complete rendering registry as
// production; a hand-picked preview list would drift and render valid tools as JSON.
export const toolPreviewPlugins: AnyPlugin[] = [
  shellPreview,
  applyPatchPreview,
  file,
  grep,
  globPreview,
  lspPreviews,
  planPreviews,
  goalPreviews,
  skillPreview,
  taskPreview,
  askUserPreview,
  webSearchPreview,
  recallPreviews,
  toolSearchPreviewPlugin,
  schedulePreview,
  httpPreviews,
];

export const toolRenderingPlugins: AnyPlugin[] = [
  ...toolPreviewPlugins,
  toolActions,
  toolViewOpener,
  toolIcons,
];

// Composer — slash commands, modes, toolbar, status chips, send & hint.

const composer: AnyPlugin[] = [
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

const panes: AnyPlugin[] = [
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
  searchView,
  agentDocsView,
  knowledgeView,
  agentMemoryView,
  inboxView,
  notificationsView,
  diagnostics,
];

// Kernel layout regions — fill the named slots in AgentClientPage.

const kernel: AnyPlugin[] = [kernelSidebar, kernelChat, kernelSettings];

// Sidebar internals — the sections that fill the expanded work-index view.

const sidebar: AnyPlugin[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

// Overlays + chrome — toasts, search, shortcuts, status, …

const overlays: AnyPlugin[] = [
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
  contextUsage,
  conversationExport,
];

export const builtinPlugins: AnyPlugin[] = [
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
