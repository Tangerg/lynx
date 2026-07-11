import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import type { ViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";
import type { IconName } from "@/ui";
import {
  AgentContextDock,
  type AgentDockTab,
  AgentDockTabs,
  AgentIconButton,
  AgentPane,
  AgentPaneHeader,
  AgentStatusPill,
  AgentToolbarButton,
} from "@/ui/agent";
import { dragClasses, noDragClasses } from "@/ui";
import { cn } from "@/lib/utils";
import { useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { basename } from "@/lib/path";
import { useActiveSession } from "@/plugins/builtin/agent/public/session";
import { useIsAgentRunning } from "@/plugins/builtin/agent/public/run";
import {
  closeWorkspaceSplit,
  closeWorkspaceView,
  openWorkspaceViewBeside,
  promoteWorkspaceSplitToView,
  useActiveWorkspaceViewId,
  useSplitWorkspaceViewId,
} from "@/plugins/builtin/workspace/public/navigation";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useSidebarRail } from "@/plugins/builtin/workspace/public/sidebarRail";
import { useUiStore } from "@/state/uiStore";
import { ChatStream } from "./ChatStream";
import { SplitResizer } from "./SplitResizer";
import { ViewPlacementProvider } from "@/plugins/builtin/workspace/public/viewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";
import { useT } from "@/lib/i18n";

const DOCK_TAB_IDS = ["diff", "file", "codebase", "terminal"];

function viewIcon(name: string | undefined): IconName | undefined {
  return name as IconName | undefined;
}

interface Props {
  onSend: (input: UserInput) => void;
}

// The dock's own top bar — the view tabs plus the collapse toggle at the far
// right (the window's top-right corner while the dock is open). Lives at the
// very top of the full-height dock column so the dock runs top-to-bottom
// independent of the center column's header.
function DockHeader({ tabs, onToggle }: { tabs: AgentDockTab[]; onToggle: () => void }) {
  const t = useT();
  return (
    <div className="flex h-[52px] shrink-0 items-center pr-2">
      <div className="min-w-0 flex-1">
        <AgentDockTabs tabs={tabs} />
      </div>
      <AgentIconButton
        icon="panel-r"
        size="sm"
        aria-label={t("workspace.view.title.context")}
        onClick={onToggle}
      />
    </div>
  );
}

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useActiveWorkspaceViewId();
  const splitViewId = useSplitWorkspaceViewId();
  const splitRatio = useUiStore((s) => s.splitRatio);
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);
  const dockCollapsed = useUiStore((s) => s.dockCollapsed);
  const toggleDock = useUiStore((s) => s.toggleDock);
  const sidebarHidden = useSidebarRail();
  const views = useWorkspaceViews();
  const { isLoading } = useAgentSessions();
  const activeSession = useActiveSession();
  const running = useIsAgentRunning();
  const t = useT();

  // Suppress the panel only while the FIRST sessions fetch is in flight (and
  // no workspace view is promoted) — avoids a blank-but-bordered flash. Once
  // loaded, render even with ZERO sessions: ChatStream shows the welcome
  // screen + composer, which is the empty-state entry point (sending there
  // spins up a session via useChatSend). Returning null on empty stranded
  // the user with a blank main area and no way to start.
  if (isLoading && !activeMainView && !splitViewId) return null;

  // Placement controls handed to a promoted view's own ViewHeader (via the
  // ViewPlacement context) so it can move itself full ↔ beside-chat / close,
  // without ChatPanel reaching into the view body or the tab strip.
  const placementFor = (id: string, placement: "full" | "split"): ViewPlacement => {
    const spec = views.find((v) => v.id === id);
    const tab = { id, title: spec?.title ?? id, icon: spec?.icon };
    return {
      placement,
      splittable: spec?.splittable ?? false,
      onSplit: () => openWorkspaceViewBeside(tab),
      onPromote: promoteWorkspaceSplitToView,
      onClose: () => (placement === "split" ? closeWorkspaceSplit() : closeWorkspaceView(id)),
    };
  };
  const dockViewId = splitViewId ?? "diff";
  const pinnedDockViews = DOCK_TAB_IDS.map((id) => views.find((view) => view.id === id)).filter(
    (view) => view !== undefined,
  );
  const activeDockView = views.find((view) => view.id === dockViewId);
  const dockViews =
    activeDockView && !pinnedDockViews.some((view) => view.id === activeDockView.id)
      ? [activeDockView, ...pinnedDockViews]
      : pinnedDockViews;
  const dockTabs = dockViews.map((view) => ({
    id: view.id,
    title: typeof view.title === "string" ? t(view.title) : view.title,
    icon: viewIcon(view.icon),
    active: view.id === dockViewId,
    onSelect: () => openWorkspaceViewBeside(view.id),
  }));
  // A split view is an explicit open → always shown; otherwise the dock follows
  // the collapse preference.
  const showDock = Boolean(splitViewId) || !dockCollapsed;

  return (
    <AgentPane tone="main" className="relative">
      {activeMainView ? (
        <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
          <WorkspaceViewBody viewId={activeMainView} />
        </ViewPlacementProvider>
      ) : (
        <div className="flex min-h-0 flex-1">
          {/* Center reading column — its own header sits above the chat stream
              and spans only this column (the dock runs full-height beside it). */}
          <div
            className={cn("relative flex min-h-0 min-w-0 flex-col", !splitViewId && "flex-1")}
            style={splitViewId ? { flexBasis: `${splitRatio * 100}%` } : undefined}
          >
            {/* Header padding (incl. the traffic-light gutter when the sidebar
                is collapsed) is owned by `.agent-pane-header` in layout.css —
                a `pl-*` utility here can't win against that unlayered rule. */}
            <AgentPaneHeader className={dragClasses}>
              <AgentIconButton
                icon="panel-l"
                size="sm"
                aria-label={
                  sidebarHidden ? t("sidebar.action.expand") : t("sidebar.action.collapse")
                }
                onClick={toggleSidebar}
                className={noDragClasses}
              />
              <span className="font-mono text-[12px] text-fg-faint">
                {activeSession?.cwd ? basename(activeSession.cwd) : "lynx"}
              </span>
              <span className="text-[13px] text-fg-faint">/</span>
              <span className="min-w-0 max-w-[320px] truncate text-[14.5px] font-semibold text-fg">
                {activeSession?.title || t("welcome.title")}
              </span>
              {running && <AgentStatusPill tone="running">运行中</AgentStatusPill>}
              <AgentIconButton
                icon="more"
                size="sm"
                aria-label="更多操作"
                className={noDragClasses}
              />
              <span className="min-w-4 flex-1" />
              <AgentToolbarButton
                icon="folder"
                trailingIcon="chevron-down"
                className={noDragClasses}
              >
                打开位置
              </AgentToolbarButton>
              {!showDock && (
                <AgentIconButton
                  icon="panel-r"
                  size="sm"
                  aria-label={t("workspace.view.title.context")}
                  onClick={toggleDock}
                  className={noDragClasses}
                />
              )}
            </AgentPaneHeader>
            <ChatStream onSend={onSend} />
          </div>
          {/* Right context dock — a full-height column with its own tab header.
              Collapsible: the header toggle hides it so the chat spans full width. */}
          {showDock &&
            (splitViewId ? (
              <>
                <SplitResizer />
                <AgentContextDock className="flex-1">
                  <DockHeader tabs={dockTabs} onToggle={toggleDock} />
                  <ViewPlacementProvider value={placementFor(splitViewId, "split")}>
                    <WorkspaceViewBody viewId={splitViewId} />
                  </ViewPlacementProvider>
                </AgentContextDock>
              </>
            ) : (
              <AgentContextDock className="w-[420px] shrink-0">
                <DockHeader tabs={dockTabs} onToggle={toggleDock} />
                <WorkspaceViewBody viewId={dockViewId} />
              </AgentContextDock>
            ))}
        </div>
      )}
    </AgentPane>
  );
}
