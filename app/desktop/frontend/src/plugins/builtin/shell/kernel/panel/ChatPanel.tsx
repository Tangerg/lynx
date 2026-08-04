import { DOCK_COLUMN, DOCK_COLUMN_COLLAPSED, dockWidthRow } from "./dockWidth";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import type { ViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
import { CatalogPicker, IconButton, type CatalogPickerGroup, type IconName } from "@/ui";
import {
  AgentContentCard,
  AgentContextDock,
  type AgentDockTab,
  AgentDockTabs,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";
import { useAgentSessions, useActiveSession } from "@/plugins/builtin/agent/public/session";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import {
  closeWorkspaceDockView,
  closeWorkspaceView,
  collapseWorkspaceDock,
  openWorkspaceViewInDock,
  selectWorkspaceDockView,
  showWorkspaceDock,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  useContextDockCatalog,
  type ContextDockDestinationGroup,
} from "@/plugins/builtin/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { basename } from "@/lib/path";
import { Slot } from "@/plugins/host/Slot";
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

function AddDockViewPicker({
  groups,
  openViewIds,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  const pickerGroups: CatalogPickerGroup[] = groups.map((group) => ({
    id: group.id,
    label: t(group.title),
    items: group.destinations.map((destination) => ({
      id: destination.viewId,
      label: t(destination.title),
      icon: viewIcon(destination.icon),
      keywords: [destination.viewId, group.id],
      active: openViewIds.has(destination.viewId),
    })),
  }));

  return (
    <CatalogPicker
      groups={pickerGroups}
      label={t("dock.action.browse")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      onSelect={(item) => openWorkspaceViewInDock(item.id)}
    />
  );
}

function DockHeader({
  tabs,
  groups,
  openViewIds,
}: {
  tabs: AgentDockTab[];
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  return (
    <AgentSurfaceHeader className="gap-1">
      <AgentDockTabs tabs={tabs} ariaLabel={t("dock.tabs.label")} />
      <AddDockViewPicker groups={groups} openViewIds={openViewIds} />
      <IconButton
        icon="panel-r"
        hoverIcon="x"
        size="sm"
        title={t("dock.action.hide")}
        onClick={collapseWorkspaceDock}
      />
    </AgentSurfaceHeader>
  );
}

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { width: dockWidth } = useDockWidth();
  const { isLoading } = useAgentSessions();
  const activeSession = useActiveSession();
  const running = useIsCurrentRootRunning();
  const t = useT();

  if (isLoading && !activeMainView && !dock.open) return null;
  // One reading of "the dock is showing", used by the resizer, the flank's own state
  // and the column width — three places that were each spelling it.
  const dockOpen = dock.open && dock.activeViewId !== null && dock.viewIds.length > 0;

  const placementFor = (id: string, placement: "full" | "dock"): ViewPlacement => ({
    placement,
    splittable: views.find((view) => view.id === id)?.splittable ?? false,
    onOpenInDock: () => openWorkspaceViewInDock(id),
    onClose: () => (placement === "dock" ? closeWorkspaceDockView(id) : closeWorkspaceView(id)),
  });

  const viewsById = new Map(views.map((view) => [view.id, view]));
  const dockTabs = dock.viewIds.map((id) => {
    const view = viewsById.get(id);
    const title = view ? t(view.title) : id;
    const Badge = view?.badge;
    return {
      id,
      title,
      icon: viewIcon(view?.icon),
      // Rendered here, not read here: the count belongs to the view, and a tab
      // strip that subscribed to every view's data to label it would re-render
      // on every diff refresh and every plan step.
      badge: Badge ? <Badge /> : undefined,
      active: id === dock.activeViewId,
      onSelect: () => selectWorkspaceDockView(id),
      onClose: () => closeWorkspaceDockView(id),
      closeLabel: `${t("common.close")} ${title}`,
    };
  });
  const openViewIds = new Set(dock.viewIds);

  return (
    <AgentContentCard label={t("shell.region.workspace")}>
      {activeMainView ? (
        <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
          <WorkspaceViewBody viewId={activeMainView} />
        </ViewPlacementProvider>
      ) : (
        <div className="flex min-h-0 flex-1" style={dockWidthRow(dockWidth)}>
          <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
            <AgentSurfaceHeader windowCorner>
              {/* Where, then what. The workspace name is the quieter half on
                  purpose: it changes rarely and only has to confirm the session
                  you are reading belongs to the checkout you think it does. */}
              {activeSession?.cwd && (
                <>
                  <span className="hidden min-w-0 max-w-[160px] shrink truncate font-mono text-ui-sm text-fg-faint lg:inline">
                    {basename(activeSession.cwd)}
                  </span>
                  <span aria-hidden className="hidden shrink-0 text-ui-sm text-fg-faint lg:inline">
                    /
                  </span>
                </>
              )}
              <span className="min-w-0 max-w-[420px] truncate text-ui-sm font-semibold text-fg">
                {activeSession?.title.trim() || t("sidebar.action.newSession")}
              </span>
              {running && (
                <AgentStatusPill tone="running">{t("session.status.running")}</AgentStatusPill>
              )}
              <span className="min-w-4 flex-1" />
              {/* Session telemetry — a number you glance at, not a control. It
                  belongs on the bar that names the session it counts, which is
                  also the one place it cannot push the transcript around. */}
              <Slot name="chat.header.meta" />
              <HeaderDiffStat />
              {!dock.open && (
                <IconButton
                  icon="panel-r"
                  size="sm"
                  title={t("dock.action.show")}
                  onClick={showWorkspaceDock}
                />
              )}
            </AgentSurfaceHeader>
            <ChatStream onSend={onSend} />
          </div>
          {dockOpen && <DockResizer />}
          {/* Always mounted, even with nothing open: a pane cannot animate in from a
              width it did not have on the previous frame, and the first view opened in
              a session is exactly the case where 0 → 336 has no starting value unless
              the element was already there at zero. */}
          {
            <AgentContextDock
              open={dockOpen}
              className="shrink-0 grow-0"
              style={dockOpen ? DOCK_COLUMN : DOCK_COLUMN_COLLAPSED}
            >
              <DockHeader tabs={dockTabs} groups={catalog} openViewIds={openViewIds} />
              <div className="relative min-h-0 flex-1">
                {dock.viewIds.map((viewId) => {
                  const active = viewId === dock.activeViewId;
                  return (
                    <div
                      key={viewId}
                      data-dock-view-id={viewId}
                      inert={active ? undefined : true}
                      aria-hidden={active ? undefined : true}
                      className={active ? "absolute inset-0 flex flex-col" : "hidden"}
                    >
                      <ViewPlacementProvider value={placementFor(viewId, "dock")}>
                        <WorkspaceViewBody viewId={viewId} />
                      </ViewPlacementProvider>
                    </div>
                  );
                })}
              </div>
            </AgentContextDock>
          }
        </div>
      )}
    </AgentContentCard>
  );
}
