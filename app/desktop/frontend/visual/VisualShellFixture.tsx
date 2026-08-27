import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { SidebarPanel } from "@/plugins/builtin/sidebar/ui/SidebarPanel";
import {
  useSidebarDrawer,
  useSidebarWidth,
} from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { Icon } from "@/ui";
import { AgentAppShell, AgentContentCard, AgentStatusPill, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualWorkIndexState } from "./shellFixtureStates";

const STATE_COPY: Record<VisualWorkIndexState, { title: string; body: string }> = {
  populated: {
    title: "A compact index for active work.",
    body: "Projects, sessions, and attention states come from production application projections.",
  },
  empty: {
    title: "Start with the work, not the chrome.",
    body: "The empty state explains how a project group is created without inventing sample data.",
  },
  loading: {
    title: "The shell remains stable while work loads.",
    body: "Only the query-owned list skeleton changes; window geometry and global actions stay ready.",
  },
  error: {
    title: "Failure stays local and actionable.",
    body: "The Work Index names the failed operation and points back to the Runtime connection.",
  },
};

export function VisualShellFixture({ state }: { state: VisualWorkIndexState }) {
  const t = useT();
  const drawer = useSidebarDrawer();
  const { width, setWidth } = useSidebarWidth();
  const copy = STATE_COPY[state];

  return (
    <AgentAppShell
      sidebarLabel={t("shell.region.workIndex")}
      sidebarResizeLabel={t("sidebar.action.resize")}
      sidebarOpen={!drawer.collapsed}
      sidebarWidth={width}
      onResize={setWidth}
      onSidebarToggle={drawer.toggle}
      sidebarExpandLabel={t("sidebar.action.expand")}
      sidebarCollapseLabel={t("sidebar.action.collapse")}
      sidebar={<SidebarPanel />}
      main={
        <AgentContentCard label="Shell and Work Index visual fixture">
          <AgentSurfaceHeader windowCorner>
            <span className="font-mono text-ui-md text-fg-faint">scope</span>
            <span className="text-ui-md text-fg-faint">/</span>
            <span className="truncate text-ui-md font-semibold text-fg">Work Index</span>
            <AgentStatusPill tone={state === "error" ? "warning" : "neutral"}>
              {state}
            </AgentStatusPill>
          </AgentSurfaceHeader>
          <div className="panel-scroll flex min-h-0 flex-1">
            <div className="m-auto flex max-w-[520px] flex-col items-center px-8 text-center">
              <span className="grid h-10 w-10 place-items-center rounded-full bg-surface-2 text-fg-muted">
                <Icon name="spark" size="md" />
              </span>
              <h1 className="mt-4 text-display-lg font-semibold leading-tight text-fg">
                {copy.title}
              </h1>
              <p className="mt-2 text-ui-md leading-relaxed text-fg-muted">{copy.body}</p>
            </div>
          </div>
        </AgentContentCard>
      }
      overlay={
        <>
          <Slot name="app.overlay" />
          <output className="sr-only" data-testid="persisted-sidebar-width">
            {width}
          </output>
          <output className="sr-only" data-testid="requested-work-index-state">
            {state}
          </output>
        </>
      }
    />
  );
}
