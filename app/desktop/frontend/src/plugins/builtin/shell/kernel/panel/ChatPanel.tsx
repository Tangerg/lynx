import { Activity } from "react";
import { dockWidthRow } from "./dockWidth";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import type { ViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
import { CatalogPicker, type CatalogPickerGroup, type IconName } from "@/ui";
import {
  AgentContentCard,
  AgentContextDock,
  type AgentDockTab,
  AgentDockTabs,
  AgentDockToggle,
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
      {activeMainView !== null && (
        <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
          <WorkspaceViewBody viewId={activeMainView} />
        </ViewPlacementProvider>
      )}
      {/* The conversation steps aside for a full-window view; it does not get torn down
          for one. `Activity` keeps it mounted and hides it (`display: none`), running
          its effect cleanups on the way out — so nothing behind Settings is still
          subscribed, and coming back is a re-show rather than a rebuild.

          Rebuilding was the visible cost: every wave and tool card you had opened
          closed itself, because that disclosure state is component state, and the whole
          transcript re-planned its render units and re-highlighted its code on the way
          back in. A ternary here cannot express any of that — an unmounted subtree has
          no state to return to. The full view above IS a ternary, deliberately: you
          come back to the conversation, not to whichever pane you passed through. */}
      <Activity mode={activeMainView === null ? "visible" : "hidden"}>
        {/* The row declares whether the flank is showing, and it is the only place that
            does. Three things answer to it — the flank travels, the conversation reflows,
            and the bar that ends up at the plane's trailing corner yields the strip the
            toggle sits in — and only the row contains all three. */}
        <div
          className="agent-dock-row flex min-h-0 flex-1"
          data-dock={dockOpen ? "open" : "collapsed"}
          style={dockWidthRow(dockWidth)}
        >
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
            </AgentSurfaceHeader>
            <ChatStream onSend={onSend} />
          </div>
          {dockOpen && <DockResizer />}
          {/* Always mounted, even with nothing open: a pane cannot travel from a place it
              did not occupy on the previous frame, and the first view opened in a session
              is exactly the case where there is no previous frame to leave unless the
              flank was already there, held outside the window by its end margin. */}
          <AgentContextDock>
            <DockHeader tabs={dockTabs} groups={catalog} openViewIds={openViewIds} />
            <div className="relative min-h-0 flex-1">
              {/* Every open tab stays mounted so switching between them keeps each
                    one's scroll, selection and expansion — but only the visible one is
                    allowed to be doing anything. `Activity` is what separates those:
                    hiding a tab runs its effect cleanups, so a diff behind another tab
                    stops watching the tree and a terminal stops following output,
                    instead of polling forever behind `display: none`.

                    It owns the hiding too, with `display: none !important` — the `inert`
                    and `aria-hidden` this used to carry were saying the same thing a
                    third and fourth time, and three mechanisms for one state is three
                    places to disagree. */}
              {dock.viewIds.map((viewId) => (
                <Activity key={viewId} mode={viewId === dock.activeViewId ? "visible" : "hidden"}>
                  <div data-dock-view-id={viewId} className="absolute inset-0 flex flex-col">
                    <ViewPlacementProvider value={placementFor(viewId, "dock")}>
                      <WorkspaceViewBody viewId={viewId} />
                    </ViewPlacementProvider>
                  </div>
                </Activity>
              ))}
            </div>
          </AgentContextDock>
          {/* Pinned to the row's trailing corner rather than placed in either bar — see
              AgentDockToggle. Last in the row so it paints over the flank it moves. */}
          <div className="agent-dock-control">
            <AgentDockToggle
              open={dockOpen}
              onToggle={dockOpen ? collapseWorkspaceDock : showWorkspaceDock}
              showLabel={t("dock.action.show")}
              hideLabel={t("dock.action.hide")}
            />
          </div>
        </div>
      </Activity>
    </AgentContentCard>
  );
}
