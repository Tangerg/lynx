import { queryClient } from "@/lib/queryClient";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/plugins/builtin/command/shortcuts";
import { useRuntimeStore } from "@/plugins/builtin/runtime/adapters/runtimeCapabilityStore";
import { kernelSettings } from "@/plugins/builtin/shell/kernel";
import appearanceSettings from "@/plugins/builtin/settings/appearance";
import providersSettings from "@/plugins/builtin/settings/providers";
import {
  EMBEDDING_ROLE_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  type ProviderConfiguration,
} from "@/plugins/builtin/settings/providers/public/queries";
import { localeEn } from "@/plugins/builtin/i18n/locales/en";
import { installWorkspaceErrorClassifier } from "@/plugins/builtin/workspace/adapters/runtimeWorkspaceErrorClassifier";
import {
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceDiff,
  type WorkspaceFileChange,
  type WorkspaceFileEntry,
  type WorkspaceFileContent,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  diffView,
  fileView,
  inboxView,
  fileTreeView,
  planView,
  terminalView,
  timelineView,
} from "@/plugins/builtin/workspace/workspace-views";
import { PENDING_WORK_KEY, type PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import {
  CONTEXT_DOCK_DESTINATION,
  DATA_PROVIDER,
  SHORTCUT,
  definePlugin,
  loadPlugin,
} from "@/plugins/sdk";
import type { PluginSpec } from "@/plugins/sdk";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import { useContextDockStore } from "@/state/contextDockStore";
import { useUiStore } from "@/state/uiStore";
import { navigator } from "@/lib/navigation";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import type { VisualWorkspaceState, VisualWorkspaceTheme } from "./workspaceFixtureStates";

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
    runReplay: { scope: "processRootSegment", maxEvents: 2_048, maxBytes: 16_777_216 },
    runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    idempotency: { retentionSeconds: 86_400 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
  },
};

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {
    // This state intentionally remains in the production query's loading path.
  });
}

function workspaceDataPlugin(state: VisualWorkspaceState): PluginSpec {
  return definePlugin({
    name: "lyra.visual.workspace-data",
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(DATA_PROVIDER, {
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
      host.extensions.contribute(DATA_PROVIDER, {
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
      host.extensions.contribute(DATA_PROVIDER, {
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
      host.extensions.contribute(DATA_PROVIDER, {
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
      host.extensions.contribute(DATA_PROVIDER, {
        key: PROVIDERS_KEY,
        fetcher: async () => PROVIDERS,
      });
      host.extensions.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async () => RESIZER_SOURCE,
      });
      host.extensions.contribute(DATA_PROVIDER, {
        key: UTILITY_ROLE_KEY,
        fetcher: async () => ({ provider: "openai", model: "gpt-5.6" }),
      });
      host.extensions.contribute(DATA_PROVIDER, {
        key: EMBEDDING_ROLE_KEY,
        fetcher: async () => ({}),
      });
    },
  });
}

const workspaceDockDestinations = definePlugin({
  name: "lyra.visual.workspace-dock-destinations",
  version: "1.0.0",
  setup({ host }) {
    for (const destination of [
      { viewId: "explorer", scope: "workspace", order: 20 },
      { viewId: "file", scope: "workspace", order: 25 },
      { viewId: "diff", scope: "workspace", order: 40 },
      { viewId: "terminal", scope: "workspace", order: 60 },
      { viewId: "plan", scope: "session", order: 120 },
      { viewId: "timeline", scope: "session", order: 140 },
      { viewId: "inbox", scope: "workspace", order: 15 },
    ] as const) {
      host.extensions.contribute(CONTEXT_DOCK_DESTINATION, destination);
    }
  },
});

const visualShortcuts = definePlugin({
  name: "lyra.visual.shortcuts",
  version: "1.0.0",
  setup({ host }) {
    for (const shortcut of [
      {
        key: "Mod+K",
        description: "shortcut.commandPalette",
        handler: () => undefined,
      },
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
      host.extensions.contribute(SHORTCUT, shortcut);
    }
  },
});

async function loadVisualPlugins(plugins: readonly PluginSpec[]): Promise<void> {
  for (const plugin of plugins) {
    const result = await loadPlugin(plugin);
    if (result.kind !== "loaded") {
      throw new Error(`Failed to install visual workspace plugin "${result.name}": ${result.kind}`);
    }
  }
}

export async function installVisualWorkspaceFixture(
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
): Promise<void> {
  await installVisualAgentFixture(state === "dock-light" ? "running" : "idle");

  installWorkspaceErrorClassifier();
  useRuntimeStore.getState().replace(VISUAL_CAPABILITIES);
  queryClient.setQueryDefaults([WORKSPACE_DIFF_KEY], {
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

  const dockViewId = state === "dock-light" ? "plan" : state === "dock-inbox" ? "inbox" : "diff";
  useContextDockStore.setState({
    activeSessionScopeId: VISUAL_SESSION_ID,
    sessionScopes: new Map(),
    // The inbox is a tab you opened, not one every workspace carries — so it is
    // present only in the state that is about it. Adding it everywhere moved the
    // tab strip in every other golden, which is a change to states that have
    // nothing to do with this feature.
    dockViewIds:
      state === "dock-inbox"
        ? ["inbox", "explorer", "file", "diff", "terminal", "plan", "timeline"]
        : ["explorer", "file", "diff", "terminal", "plan", "timeline"],
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
    dockWidth: 520,
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
