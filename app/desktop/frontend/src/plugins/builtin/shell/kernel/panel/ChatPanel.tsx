import { DOCK_COLUMN, dockWidthRow } from "./dockWidth";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import type { ViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
import type { IconName } from "@/ui";
import {
  AgentContentCard,
  AgentContextDock,
  type AgentDockTab,
  AgentDockTabs,
  AgentDrawerToggle,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";
import { IconButton } from "@/ui";
import { useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { basename } from "@/lib/path";
import { useActiveSession } from "@/plugins/builtin/agent/public/session";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import {
  closeWorkspaceDockView,
  closeWorkspaceView,
  openContextDockLauncher,
  openWorkspaceViewInDock,
  promoteWorkspaceDockViewToFull,
  toggleContextDock,
  useActiveWorkspaceViewId,
  useDockWorkspaceViewId,
} from "@/plugins/builtin/workspace/public/navigation";
import { useContextDockPinned } from "@/plugins/builtin/workspace/public/contextDockTabs";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useDockWidth, useSidebarDrawer } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { ChatStream } from "./ChatStream";
import { DockResizer } from "./DockResizer";
import { HeaderDiffStat } from "./HeaderDiffStat";
import { ViewPlacementProvider } from "@/plugins/builtin/workspace/public/viewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";
import { useT } from "@/lib/i18n";

function viewIcon(name: string | undefined): IconName | undefined {
  return name as IconName | undefined;
}

interface Props {
  onSend: (input: UserInput) => void;
}

// The dock's own bar: the pinned view chips plus the controls that belong to the
// dock as a container — browse everything else, hand the view the whole card,
// hide the dock. The view inside contributes its subtext and its own actions, but
// not a second title: the chip already names it.
function DockHeader({
  tabs,
  onBrowse,
  onMaximize,
  onHide,
}: {
  tabs: AgentDockTab[];
  onBrowse: () => void;
  onMaximize: () => void;
  onHide: () => void;
}) {
  const t = useT();
  return (
    <AgentSurfaceHeader className="gap-1">
      <AgentDockTabs tabs={tabs} />
      <IconButton icon="more" size="sm" aria-label={t("dock.action.browse")} onClick={onBrowse} />
      <IconButton
        icon="maximize"
        size="sm"
        aria-label={t("workspace.view.promote")}
        onClick={onMaximize}
      />
      <IconButton icon="panel-r" size="sm" aria-label={t("dock.action.hide")} onClick={onHide} />
    </AgentSurfaceHeader>
  );
}

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useActiveWorkspaceViewId();
  const dockViewId = useDockWorkspaceViewId();
  const drawer = useSidebarDrawer();
  const pinnedDockViews = useContextDockPinned();
  const views = useWorkspaceViews();
  // The dock is as wide as its material needs. The view in it declares that, so
  // the width is read from — and a drag written back to — the density's own slot.
  const dockDensity = views.find((view) => view.id === dockViewId)?.density ?? "light";
  const { width: dockWidth } = useDockWidth(dockDensity);
  const { isLoading } = useAgentSessions();
  const activeSession = useActiveSession();
  const running = useIsCurrentRootRunning();
  const t = useT();

  // Suppress the panel only while the FIRST sessions fetch is in flight (and
  // no workspace view is open) — avoids a blank-but-bordered flash. Once
  // loaded, render even with ZERO sessions: ChatStream shows the welcome
  // screen + composer, which is the empty-state entry point (sending there
  // spins up a session via useChatSend). Returning null on empty stranded
  // the user with a blank main area and no way to start.
  if (isLoading && !activeMainView && !dockViewId) return null;

  // Placement controls handed to a view's own ViewHeader (via the ViewPlacement
  // context) so it can move itself full ↔ dock / close, without ChatPanel
  // reaching into the view body.
  const placementFor = (id: string, placement: "full" | "dock"): ViewPlacement => ({
    placement,
    splittable: views.find((view) => view.id === id)?.splittable ?? false,
    onOpenInDock: () => openWorkspaceViewInDock(id),
    onClose: () => (placement === "dock" ? closeWorkspaceDockView() : closeWorkspaceView(id)),
  });

  // The active view keeps a chip while it is open even when it is not pinned, so
  // the strip always shows where you are.
  const dockTabSources =
    dockViewId && !pinnedDockViews.some((view) => view.viewId === dockViewId)
      ? [
          ...views
            .filter((view) => view.id === dockViewId)
            .map((view) => ({ viewId: view.id, title: view.title, icon: view.icon })),
          ...pinnedDockViews,
        ]
      : pinnedDockViews;
  const dockTabs = dockTabSources.map((view) => ({
    id: view.viewId,
    title: t(view.title),
    icon: viewIcon(view.icon),
    active: view.viewId === dockViewId,
    onSelect: () => openWorkspaceViewInDock(view.viewId),
  }));

  return (
    <AgentContentCard label={t("shell.region.workspace")}>
      {activeMainView ? (
        <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
          <WorkspaceViewBody viewId={activeMainView} />
        </ViewPlacementProvider>
      ) : (
        <div className="flex min-h-0 flex-1" style={dockWidthRow(dockWidth)}>
          {/* Center reading column — its own header sits above the chat stream
              and spans only this column (the dock runs full-height beside it). */}
          <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
            {/* Height, inset, the traffic-light gutter and the window drag region
                are owned by `.agent-surface-header` in globals.css — a `pl-*`
                utility here can't win against that unlayered rule. */}
            <AgentSurfaceHeader windowCorner>
              {drawer.collapsed && (
                <AgentDrawerToggle
                  collapsed
                  onToggle={drawer.toggle}
                  expandLabel={t("sidebar.action.expand")}
                  collapseLabel={t("sidebar.action.collapse")}
                />
              )}
              <span className="font-mono text-ui-md text-fg-faint">
                {activeSession?.cwd ? basename(activeSession.cwd) : "lynx"}
              </span>
              <span className="text-ui-lg text-fg-faint">/</span>
              <span className="min-w-0 max-w-[320px] truncate text-ui-lg font-semibold text-fg">
                {activeSession?.title || t("welcome.title")}
              </span>
              {running && (
                <AgentStatusPill tone="running">{t("session.status.running")}</AgentStatusPill>
              )}
              <span className="min-w-4 flex-1" />
              <HeaderDiffStat />
              {!dockViewId && (
                <IconButton
                  icon="panel-r"
                  size="sm"
                  aria-label={t("dock.action.show")}
                  onClick={toggleContextDock}
                />
              )}
            </AgentSurfaceHeader>
            <ChatStream onSend={onSend} />
          </div>
          {/* Right context dock — a full-height, resizable column with its own
              bar. `dockViewId` alone says whether it is open: there is no second
              collapse flag that could disagree and leave a toggle inert. */}
          {dockViewId && (
            <>
              <DockResizer density={dockDensity} />
              <AgentContextDock className="shrink-0 grow-0" style={DOCK_COLUMN}>
                <DockHeader
                  tabs={dockTabs}
                  onBrowse={openContextDockLauncher}
                  onMaximize={promoteWorkspaceDockViewToFull}
                  onHide={closeWorkspaceDockView}
                />
                <ViewPlacementProvider value={placementFor(dockViewId, "dock")}>
                  <WorkspaceViewBody viewId={dockViewId} />
                </ViewPlacementProvider>
              </AgentContextDock>
            </>
          )}
        </div>
      )}
    </AgentContentCard>
  );
}
