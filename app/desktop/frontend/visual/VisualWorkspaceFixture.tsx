import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { PluginToaster } from "@/plugins/host/PluginToaster";
import {
  useActiveWorkspaceViewId,
  useWorkspaceDock,
} from "@/plugins/builtin/workspace/public/navigation";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualWorkspaceState } from "./workspaceFixtureStates";

const STATE_LABELS: Record<VisualWorkspaceState, string> = {
  "dock-light": "Plan workspace",
  "dock-review": "Diff review",
  "dock-inbox": "Inbox",
  "dock-stats": "Tool stats",
  "dock-tools": "Tool catalog",
  "dock-file": "File viewer",
  "dock-empty": "Diff · empty",
  "dock-loading": "Diff · loading",
  "dock-error": "Diff · error",
  settings: "Settings",
};

function WorkspaceStateSidebar({ state }: { state: VisualWorkspaceState }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-md font-semibold text-fg">Workspace states</span>
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
  const dock = useWorkspaceDock();
  const activeMainViewId = useActiveWorkspaceViewId();
  const dockWidth = useDockWidth().width;

  return (
    <>
      <output className="sr-only" data-testid="requested-workspace-state">
        {state}
      </output>
      <output className="sr-only" data-testid="active-dock-view">
        {dock.activeViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="dock-open">
        {String(dock.open)}
      </output>
      <output className="sr-only" data-testid="dock-view-ids">
        {dock.viewIds.join(",")}
      </output>
      <output className="sr-only" data-testid="active-main-view">
        {activeMainViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="persisted-dock-width">
        {dockWidth}
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
      sidebarWidth={SIDEBAR_DEFAULT_WIDTH_PX}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the workspace fixture sidebar"
      sidebarCollapseLabel="Collapse the workspace fixture sidebar"
      sidebar={settingsOpen ? undefined : <WorkspaceStateSidebar state={state} />}
      main={
        <div className="contents" data-testid="workspace-state" data-state={state}>
          <ChatPanel onSend={() => true} />
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
