import { queryClient } from "@/lib/queryClient";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/plugins/builtin/command/shortcuts";
import { useRuntimeConnectionStore } from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import { kernelSettings } from "@/plugins/builtin/shell/kernel";
import appearanceSettings from "@/plugins/builtin/settings/appearance";
import providersSettings from "@/plugins/builtin/settings/providers";
import {
  EMBEDDING_ROLE_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  type ProviderConfiguration,
} from "@/plugins/builtin/settings/providers/public/queries";
import {
  MCP_SERVERS_KEY,
  type MCPServerSettings,
} from "@/plugins/builtin/settings/mcp-servers/public/queries";
import { localeEn } from "@/plugins/builtin/i18n/locales/en";
import { installWorkspaceErrorClassifier } from "@/plugins/builtin/workspace/adapters/runtimeWorkspaceErrorClassifier";
import {
  WORKSPACE_BUILTIN_TOOLS_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type BuiltinToolSummary,
  type WorkspaceDiff,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  diffView,
  fileView,
  inboxView,
  fileTreeView,
  planView,
  terminalView,
  timelineView,
  toolStatsView,
  toolsView,
} from "@/plugins/builtin/workspace/workspace-views";
import { builtinContextDockDestinations } from "@/plugins/builtin/workspace/application/contextDockDestinations";
import { PENDING_WORK_KEY, type PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import { CONTEXT_DOCK_DESTINATION, DATA_PROVIDER, SHORTCUT, definePlugin } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import { useContextDockStore } from "@/state/contextDockStore";
import { useUiStore } from "@/state/uiStore";
import { navigator } from "@/lib/navigation";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import {
  VISUAL_DOCK_WIDTH_PX,
  VISUAL_REVIEW_DOCK_WIDTH_PX,
  type VisualWorkspaceState,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

const ACTIVE_DIFF_FILE =
  "app/desktop/frontend/src/plugins/builtin/shell/kernel/panel/DockResizer.tsx";

const REVIEW_DIFF: WorkspaceDiff = {
  files: [
    {
      path: ACTIVE_DIFF_FILE,
      status: "modified",
      added: 18,
      removed: 6,
      rows: [
        { type: "hunk", text: "@@ -104,8 +104,16 @@ export function DockResizer" },
        {
          type: "context",
          leftLine: 104,
          rightLine: 104,
          code: "const rail = railRef.current;",
        },
        {
          type: "deleted",
          leftLine: 105,
          code: "const width = clampDockWidth(persistedWidth, row.clientWidth);",
        },
        {
          type: "added",
          rightLine: 105,
          code: "const currentWidth = readDockWidth(row);",
        },
        {
          type: "added",
          rightLine: 106,
          code: "const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
        },
        {
          type: "context",
          leftLine: 106,
          rightLine: 107,
          code: "row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
        },
        {
          type: "added",
          rightLine: 108,
          code: 'rail.setAttribute("aria-valuenow", String(nextWidth));',
        },
      ],
    },
    {
      path: "app/runtime/protocol/session.go",
      status: "modified",
      added: 7,
      removed: 3,
      rows: [
        { type: "hunk", text: "@@ -48,5 +48,7 @@ func (session *Session) Commit" },
        {
          type: "context",
          leftLine: 48,
          rightLine: 48,
          code: "func (session *Session) Commit(ctx context.Context) error {",
        },
        {
          type: "deleted",
          leftLine: 49,
          code: "\treturn session.store.Save(ctx, session)",
        },
        {
          type: "added",
          rightLine: 49,
          code: "\tsnapshot := session.snapshot()",
        },
        {
          type: "added",
          rightLine: 50,
          code: "\treturn session.records.Commit(ctx, snapshot)",
        },
        { type: "context", leftLine: 50, rightLine: 51, code: "}" },
      ],
    },
  ],
};

const RESIZER_SOURCE: WorkspaceFileContent = {
  totalLines: 8,
  content: [
    "const onKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {",
    '  if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;',
    "  const row = railRef.current?.parentElement;",
    "  if (!row) return;",
    "  const currentWidth = readDockWidth(row);",
    "  const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
    "  row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
    "}, [setWidth]);",
  ].join("\n"),
};

const PROVIDERS: ProviderConfiguration[] = [
  {
    id: "openai",
    baseUrl: "https://api.openai.com/v1",
    apiKeyMasked: "sk-…7F2A",
    keySource: "stored",
    embeddingCapable: true,
    defaultEmbeddingModel: "text-embedding-3-large",
  },
  {
    id: "anthropic",
    baseUrl: "https://api.anthropic.com",
    apiKeyMasked: "",
    embeddingCapable: false,
  },
];

function stable(enabled: boolean): FeatureCapability {
  return { enabled, stability: "stable", clientOptIn: false, requiredByRunProtocol: false };
}

const VISUAL_CAPABILITIES: ServerCapabilities = {
  runEvents: [],
  runtimeTopics: [],
  stateSnapshots: [],
  features: { git: stable(true), plan: stable(true) },
  streamingMethods: [],
  limits: {
    runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2_048, maxBytes: 16_777_216 },
    runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    idempotency: { namespace: "idp_visual_fixture", retentionSeconds: 86_400 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
  },
};

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {
    // This state intentionally remains in the production query's loading path.
  });
}

function workspaceDataPlugin(state: VisualWorkspaceState): AnyPlugin {
  return definePlugin({
    name: "lyra.visual.workspace-data",
    setup(ctx) {
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_DIFF_KEY,
        fetcher: async () => {
          if (state === "dock-loading") return pending<WorkspaceDiff>();
          if (state === "dock-error") {
            throw new Error("Visual fixture could not load the workspace diff");
          }
          if (state === "dock-empty") return { files: [] };
          return REVIEW_DIFF;
        },
      });
      // The churn summary the header stat and the Diff tab's badge both read.
      // It is a separate query from the diff itself — the diff is what you are
      // looking at, this is what changed — so the fixture has to answer both or
      // two production readouts stay invisible to every screenshot.
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_FILES_CHANGED_KEY,
        fetcher: async () => {
          if (state === "dock-loading") return pending<WorkspaceFileChange[]>();
          if (state === "dock-error") {
            throw new Error("Visual fixture could not load the workspace file changes");
          }
          if (state === "dock-empty") return [];
          return [
            {
              path: "app/desktop/frontend/src/plugins/DockResizer.tsx",
              change: "mod",
              added: 18,
              removed: 6,
            },
            { path: "app/runtime/protocol/session.go", change: "mod", added: 7, removed: 3 },
          ] satisfies WorkspaceFileChange[];
        },
      });
      // The catalog the Tools view groups. A subset, but a subset that spans the
      // shapes: every safety class, a family with one member and a family with
      // several, and a name the local family table has never heard of — which is
      // what proves an unplaced tool still lists instead of vanishing.
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_BUILTIN_TOOLS_KEY,
        fetcher: async () =>
          [
            {
              name: "shell",
              description: "Run a shell command",
              parameters: { type: "object", required: ["command"] },
              safetyClass: "exec",
            },
            {
              name: "read_shell_output",
              description: "Read command output",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "read",
              description: "Read a file",
              parameters: { type: "object", required: ["path"] },
              safetyClass: "safe",
            },
            {
              name: "apply_patch",
              description: "Apply a patch to files",
              parameters: { type: "object" },
              safetyClass: "write",
            },
            {
              name: "grep",
              description: "Search file contents",
              parameters: { type: "object", required: ["query"] },
              safetyClass: "safe",
            },
            {
              name: "glob",
              description: "Find files by name",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "web_fetch",
              description: "Fetch a page",
              parameters: { type: "object", required: ["url"] },
              safetyClass: "network",
            },
            {
              name: "set_plan",
              description: "Update the Plan",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "search_memory",
              description: "Search project memory",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "acme_deploy",
              description: "Ship it (unplaced, from a plugin)",
              parameters: { type: "object" },
            },
          ] satisfies BuiltinToolSummary[],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: MCP_SERVERS_KEY,
        fetcher: async () => [] satisfies MCPServerSettings[],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_LIST_FILES_KEY,
        fetcher: async () =>
          [
            { path: "app", name: "app", type: "dir" },
            { path: "go.mod", name: "go.mod", type: "file", sizeBytes: 4_096 },
            { path: "README.md", name: "README.md", type: "file", sizeBytes: 2_048 },
          ] satisfies WorkspaceFileEntry[],
      });
      // Two sessions blocked on two different kinds of ask, one of them on a
      // batch — the shapes the row has to tell apart. An empty queue is the
      // other states' job, so this one is never empty.
      ctx.contribute(DATA_PROVIDER, {
        key: PENDING_WORK_KEY,
        fetcher: async () =>
          state === "dock-inbox"
            ? ([
                {
                  id: "ses_visual:run_root",
                  sessionId: VISUAL_SESSION_ID,
                  rootRunId: "run_root",
                  kind: "approval",
                  subject: "shell",
                  more: 2,
                  waitingSince: "2026-07-31T07:52:00.000Z",
                },
                {
                  id: "ses_other:run_b",
                  sessionId: "ses_other",
                  rootRunId: "run_b",
                  kind: "question",
                  subject: "Which database should the migration target?",
                  more: 0,
                  waitingSince: "2026-07-31T07:58:00.000Z",
                },
              ] satisfies PendingWorkItem[])
            : [],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: PROVIDERS_KEY,
        fetcher: async () => PROVIDERS,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async () => RESIZER_SOURCE,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: UTILITY_ROLE_KEY,
        fetcher: async () => ({ provider: "openai", model: "gpt-5.6" }),
      });
      ctx.contribute(DATA_PROVIDER, {
        key: EMBEDDING_ROLE_KEY,
        fetcher: async () => ({}),
      });
    },
  });
}

/**
 * Which views this fixture loads — the ONLY thing it decides about the dock.
 *
 * Scope and order come from the production catalog, filtered to these. A
 * hand-written destination list here is what let two views be photographed as
 * reachable while the product had no destination for either: the fixture invented
 * the entry that production was missing. Selecting from the real catalog cannot
 * invent one, and a view the catalog forgets now goes missing from the golden
 * instead of passing.
 */
const FIXTURE_VIEW_IDS = new Set([
  "explorer",
  "file",
  "diff",
  "terminal",
  "plan",
  "timeline",
  "inbox",
  "tool-stats",
]);

const workspaceDockDestinations = definePlugin({
  name: "lyra.visual.workspace-dock-destinations",
  setup(ctx) {
    for (const destination of builtinContextDockDestinations) {
      if (!FIXTURE_VIEW_IDS.has(destination.viewId)) continue;
      ctx.contribute(CONTEXT_DOCK_DESTINATION, destination);
    }
  },
});

const visualShortcuts = definePlugin({
  name: "lyra.visual.shortcuts",
  setup(ctx) {
    for (const shortcut of [
      {
        key: "Mod+N",
        description: "sidebar.action.newSession",
        handler: () => undefined,
      },
      {
        key: "Escape",
        description: "shortcut.closeWorkspaceView",
        handler: () => undefined,
      },
    ]) {
      ctx.contribute(SHORTCUT, shortcut);
    }
  },
});

async function loadVisualPlugins(plugins: readonly AnyPlugin[]): Promise<void> {
  await loadPluginsForTest(...plugins);
}

// Which dock view each state is ABOUT. A state not named here is a diff state —
// there are four of them and they differ in their data, not in their destination.
const DOCK_VIEW_BY_STATE: Partial<Record<VisualWorkspaceState, string>> = {
  "dock-light": "plan",
  "dock-inbox": "inbox",
  "dock-stats": "tool-stats",
  "dock-tools": "tools",
  "dock-file": "file",
};

export async function installVisualWorkspaceFixture(
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
): Promise<void> {
  // Tool stats needs a session that actually ran tools; every other state wants
  // the quiet one. `tool-shells` is the state with a read, a command, an edit, a
  // failure and a refusal in it — five outcomes, which is what the view sorts.
  await installVisualAgentFixture(
    state === "dock-light" ? "running" : state === "dock-stats" ? "tool-shells" : "idle",
  );

  installWorkspaceErrorClassifier();
  useRuntimeConnectionStore.setState({ capabilities: VISUAL_CAPABILITIES });
  queryClient.setQueryDefaults([WORKSPACE_DIFF_KEY], {
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

  const dockViewId = DOCK_VIEW_BY_STATE[state] ?? "diff";
  useContextDockStore.setState({
    activeSessionScopeId: VISUAL_SESSION_ID,
    sessionScopes: new Map(),
    // The inbox is a tab you opened, not one every workspace carries — so it is
    // present only in the state that is about it. Adding it everywhere moved the
    // tab strip in every other golden, which is a change to states that have
    // nothing to do with this feature.
    // A view this feature added is a tab you opened, not one every workspace
    // carries. Present only in the state that is about it: adding it everywhere
    // moved the tab strip in every other golden, which is a change to states
    // that have nothing to do with the feature.
    dockViewIds: [
      ...(DOCK_VIEW_BY_STATE[state] === "inbox" ? ["inbox"] : []),
      ...(DOCK_VIEW_BY_STATE[state] === "tool-stats" ? ["tool-stats"] : []),
      ...(DOCK_VIEW_BY_STATE[state] === "tools" ? ["tools"] : []),
      "explorer",
      "file",
      "diff",
      "terminal",
      "plan",
      "timeline",
    ],
    lastViewId: dockViewId,
    activeFile: ACTIVE_DIFF_FILE,
    fileViewer: { path: ACTIVE_DIFF_FILE, line: 6 },
    selectedToolId: "",
    expandedToolIds: new Set(),
  });
  // The dock is open because the location names a destination — there is no
  // separate flag to set, which is the point.
  navigator().go({
    session: VISUAL_SESSION_ID,
    dock: dockViewId,
    view: state === "settings" ? "settings" : null,
    settings: state === "settings" ? "appearance" : null,
  });
  useUiStore.setState({
    theme,
    visualStyle: "lyra",
    motionScale: 0,
    sidebarCollapsed: false,
    sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
    // Only the review view splits, and only above a width the others never need.
    dockWidth: state === "dock-review" ? VISUAL_REVIEW_DOCK_WIDTH_PX : VISUAL_DOCK_WIDTH_PX,
  });

  // The palettes and the visual style come from the agent installer above — this
  // fixture builds ON it, and loading a plugin twice reports `skipped`, which the
  // loader here treats as a failure.
  await loadVisualPlugins([
    workspaceDataPlugin(state),
    workspaceDockDestinations,
    diffView,
    fileView,
    fileTreeView,
    inboxView,
    terminalView,
    toolStatsView,
    toolsView,
    planView,
    timelineView,
    kernelSettings,
    localeEn,
    appearanceSettings,
    providersSettings,
    shortcutsSettings,
    visualShortcuts,
  ]);

  const root = document.documentElement;
  root.dataset.visualDockWidthCommits = "0";
  useUiStore.subscribe((next, previous) => {
    if (next.dockWidth === previous.dockWidth) return;
    root.dataset.visualDockWidthCommits = String(
      Number(root.dataset.visualDockWidthCommits ?? "0") + 1,
    );
  });
}
