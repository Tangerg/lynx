import { DOCK_COLUMN, dockWidthRow } from "./dockWidth";
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
    return {
      id,
      title,
      icon: viewIcon(view?.icon),
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
              <span className="min-w-0 max-w-[420px] truncate text-ui-lg font-medium text-fg">
                {activeSession?.title || t("welcome.title")}
              </span>
              {running && (
                <AgentStatusPill tone="running">{t("session.status.running")}</AgentStatusPill>
              )}
              <span className="min-w-4 flex-1" />
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
          {dock.open && dock.activeViewId && <DockResizer />}
          {dock.viewIds.length > 0 && (
            <AgentContextDock
              className="shrink-0 grow-0"
              style={{
                ...DOCK_COLUMN,
                display: dock.open && dock.activeViewId ? undefined : "none",
              }}
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
          )}
        </div>
      )}
    </AgentContentCard>
  );
}
