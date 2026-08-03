import { queryClient } from "@/lib/queryClient";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import {
  sidebarActions,
  sidebarFooter,
  sidebarProjects,
  sidebarRecents,
} from "@/plugins/builtin/sidebar";
import { builtinVisualStyles } from "@/plugins/builtin/theme/visualStyles";
import lyraDark from "@/plugins/builtin/theme/themes/lyra-dark";
import lyraLight from "@/plugins/builtin/theme/themes/lyra-light";
import { defaultAccents } from "@/plugins/builtin/defaults";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";
import {
  WORKSPACE_PROJECTS_KEY,
  type WorkspaceProjectSummary,
} from "@/plugins/builtin/workspace/public/queries";
import { DATA_PROVIDER, definePlugin, loadPlugin, usePluginStore } from "@/plugins/sdk";
import type { PluginSpec } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import type { VisualShellTheme, VisualWorkIndexState } from "./shellFixtureStates";

const ACTIVE_SESSION_ID = "visual-shell-active";

const PROJECTS: WorkspaceProjectSummary[] = [
  {
    id: "/Users/visual/lynx",
    name: "lynx",
    branch: "main",
    sessionCount: 6,
  },
  {
    id: "/Users/visual/runtime",
    name: "runtime-lab",
    branch: "protocol-vnext",
    sessionCount: 2,
  },
];

const SESSIONS: AgentSessionSummary[] = [
  session(
    ACTIVE_SESSION_ID,
    "Refine Runtime protocol",
    "idle",
    "/Users/visual/lynx",
    "14:29",
    true,
  ),
  session(
    "visual-shell-running",
    "Align desktop shell and Work Index",
    "running",
    "/Users/visual/lynx",
    "14:18",
  ),
  session(
    "visual-shell-waiting",
    "Review cancellation ownership",
    "waiting",
    "/Users/visual/lynx",
    "11:30",
  ),
  session(
    "visual-shell-long",
    "Validate a deliberately long mixed CJK / English session title without breaking the row",
    "idle",
    "/Users/visual/lynx",
    "2026-07-30T14:30:00Z",
  ),
  session(
    "visual-shell-checks",
    "Close conformance gates",
    "idle",
    "/Users/visual/lynx",
    "2026-07-29T14:30:00Z",
  ),
  session(
    "visual-shell-more",
    "Remove compatibility residue",
    "idle",
    "/Users/visual/lynx",
    "2026-07-28T14:30:00Z",
  ),
  session(
    "visual-shell-runtime",
    "Trace replay recovery",
    "idle",
    "/Users/visual/runtime",
    "2026-07-27T14:30:00Z",
  ),
  // No project claims these two, so they are what the Recent section renders:
  // one started in a scratch directory, one started before a folder was picked.
  session(
    "visual-shell-scratch",
    "Inspect an unregistered checkout",
    "idle",
    "/Users/visual/scratch",
    "2026-07-25T14:30:00Z",
  ),
  session("visual-shell-adrift", "Draft a release note", "idle", "", "2026-07-24T14:30:00Z"),
];

function session(
  id: string,
  title: string,
  status: AgentSessionSummary["status"],
  cwd: string,
  time: string,
  favorite = false,
): AgentSessionSummary {
  return {
    id,
    revision: 7,
    title,
    status,
    model: "gpt-5.6",
    cwd,
    favorite,
    time: time.includes("T") ? time : `2026-07-31T${time}:00Z`,
  };
}

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {
    // A loading fixture intentionally never settles.
  });
}

function dataProviderPlugin(state: VisualWorkIndexState): PluginSpec {
  return definePlugin({
    name: "lyra.visual.work-index-data",
    version: "1.0.0",
    setup({ host }) {
      host.extensions.contribute(DATA_PROVIDER, {
        key: WORKSPACE_PROJECTS_KEY,
        fetcher: async () => {
          if (state === "loading") return pending<WorkspaceProjectSummary[]>();
          if (state === "error") {
            throw new Error("Visual fixture Runtime connection unavailable");
          }
          return state === "empty" ? [] : PROJECTS;
        },
      });
      host.extensions.contribute(DATA_PROVIDER, {
        key: AGENT_SESSIONS_KEY,
        fetcher: async () => (state === "populated" ? SESSIONS : []),
      });
    },
  });
}

export async function installVisualShellFixture(
  state: VisualWorkIndexState,
  theme: VisualShellTheme,
  sidebarOpen: boolean,
): Promise<void> {
  usePluginStore.getState().resetForTest();
  queryClient.clear();
  queryClient.setQueryDefaults([WORKSPACE_PROJECTS_KEY], { retry: false });
  queryClient.setQueryDefaults([AGENT_SESSIONS_KEY], { retry: false });

  installAgentStatePorts();
  installWorkspaceNavigationPort();
  useAgentSessionStore.setState({
    activeSessionId: state === "populated" ? ACTIVE_SESSION_ID : "",
    openSessionIds: state === "populated" ? [ACTIVE_SESSION_ID] : [],
    draftSessionIds: new Set(),
    pendingMessages: {},
  });
  useUiStore.setState({
    theme,
    visualStyle: "lyra",
    sidebarCollapsed: !sidebarOpen,
    sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
  });

  // The visual styles ship the shell's whole material vocabulary — region fills,
  // seam, casts, radii, control heights. Without them registered the suite runs on
  // the globals.css fallbacks, so every screenshot here would be of a skin the
  // product never renders and no style regression could ever fail a test.
  for (const plugin of [
    dataProviderPlugin(state),
    lyraLight,
    defaultAccents,
    lyraDark,
    ...builtinVisualStyles,
    sidebarActions,
    sidebarProjects,
    sidebarRecents,
    sidebarFooter,
  ]) {
    const result = await loadPlugin(plugin);
    if (result.kind !== "loaded") {
      throw new Error(`Failed to install visual shell plugin "${result.name}": ${result.kind}`);
    }
  }
}
