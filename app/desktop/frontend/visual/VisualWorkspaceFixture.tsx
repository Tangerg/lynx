import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { PluginToaster } from "@/plugins/host/PluginToaster";
import {
  useActiveWorkspaceViewId,
  useDockWorkspaceViewId,
} from "@/plugins/builtin/workspace/public/navigation";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualWorkspaceState } from "./workspaceFixtureStates";

const STATE_LABELS: Record<VisualWorkspaceState, string> = {
  "dock-light": "Plan · light",
  "dock-review": "Diff · review",
  "dock-empty": "Diff · empty",
  "dock-loading": "Diff · loading",
  "dock-error": "Diff · error",
  settings: "Settings",
};

function WorkspaceStateSidebar({ state }: { state: VisualWorkspaceState }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-lg font-semibold text-fg">Workspace states</span>
      </AgentSurfaceHeader>
      <div className="flex flex-col gap-0.5 px-2 pt-2">
        {(Object.keys(STATE_LABELS) as VisualWorkspaceState[]).map((candidate) => (
          <AgentRow
            key={candidate}
            icon={
              candidate === "settings"
                ? "settings"
                : candidate.includes("error")
                  ? "alert"
                  : "panel-r"
            }
            active={candidate === state}
          >
            {STATE_LABELS[candidate]}
          </AgentRow>
        ))}
      </div>
      <div className="min-h-4 flex-1" />
      <div className="px-4 pb-3 text-ui-xs leading-body text-fg-faint">
        Production views · deterministic providers
      </div>
    </div>
  );
}

function WorkspaceFixtureReadout({ state }: { state: VisualWorkspaceState }) {
  const dockViewId = useDockWorkspaceViewId();
  const activeMainViewId = useActiveWorkspaceViewId();
  const lightWidth = useDockWidth("light").width;
  const reviewWidth = useDockWidth("review").width;

  return (
    <>
      <output className="sr-only" data-testid="requested-workspace-state">
        {state}
      </output>
      <output className="sr-only" data-testid="active-dock-view">
        {dockViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="active-main-view">
        {activeMainViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="persisted-light-dock-width">
        {lightWidth}
      </output>
      <output className="sr-only" data-testid="persisted-review-dock-width">
        {reviewWidth}
      </output>
    </>
  );
}

export function VisualWorkspaceFixture({ state }: { state: VisualWorkspaceState }) {
  const settingsOpen = state === "settings";

  return (
    <AgentAppShell
      sidebarLabel="Workspace fixture states"
      sidebarResizeLabel="Resize the workspace fixture sidebar"
      sidebarOpen={!settingsOpen}
      sidebarWidth={256}
      onResize={() => undefined}
      sidebar={settingsOpen ? undefined : <WorkspaceStateSidebar state={state} />}
      main={
        <div className="contents" data-testid="workspace-state" data-state={state}>
          <ChatPanel onSend={() => undefined} />
        </div>
      }
      overlay={
        <>
          <WorkspaceFixtureReadout state={state} />
          <PluginToaster />
        </>
      }
    />
  );
}
