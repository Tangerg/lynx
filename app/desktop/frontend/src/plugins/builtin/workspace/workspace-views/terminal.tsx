import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { workspaceCommandActivitiesFromAgentTools } from "../application/toolActivity";
import { TerminalViewModel, terminalSubtext } from "../application/terminalViewModel";
import { CommandLog } from "./views/CommandLog";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useSelectedWorkspaceToolId } from "@/plugins/builtin/workspace/public/navigation";

// The agent's command log (G5). Each command's output streams via
// item.delta{toolOutput} → item.completed — 613 confirmed that's already on
// the wire (no new API), and the run fold lands it in view.toolCalls. This view
// just consolidates the command-category tools into one terminal-like surface.
// (A user-interactive PTY is deliberately out of the runtime's scope, so this is
// a read-only log of what the agent ran, not an input terminal.)
export function TerminalWorkspaceSurface() {
  const t = useT();
  const toolCalls = useActiveSessionToolCalls();
  const selectedToolId = useSelectedWorkspaceToolId();
  const view = useMemo(
    () => TerminalViewModel.from(workspaceCommandActivitiesFromAgentTools(toolCalls)),
    [toolCalls],
  );
  const selectedCommandId = view.selectedCommandId(selectedToolId);

  // Terminal semantics: open at the bottom (latest command) and tail live
  // output — but only while the user is pinned to the bottom, so scrolling up
  // to read an earlier command isn't fought. Lightweight stick-to-bottom off
  // the view's shared scroll container (no extra lib for a read-only log).
  const scrollRef = useRef<HTMLDivElement>(null);
  const pinnedRef = useRef(true);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);
  useLayoutEffect(() => {
    if (!selectedCommandId) {
      pinnedRef.current = true;
      return;
    }
    pinnedRef.current = view.latestCommandId === selectedCommandId;
    scrollRef.current
      ?.querySelector<HTMLElement>("[data-command-selected]")
      ?.scrollIntoView?.({ block: "nearest" });
  }, [selectedCommandId, view.latestCommandId]);
  useEffect(() => {
    if (!pinnedRef.current) return;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [view]);

  return (
    <WorkspaceViewLayout
      icon="terminal"
      title="terminal.title"
      sub={terminalSubtext(t, view)}
      scrollRef={scrollRef}
    >
      {view.isEmpty ? (
        <EmptyState
          icon="terminal"
          title={t("terminal.empty.title")}
          sub={t("terminal.empty.sub")}
        />
      ) : (
        <CommandLog commands={view.commands} selectedCommandId={selectedCommandId} />
      )}
    </WorkspaceViewLayout>
  );
}

export const terminalView = defineWorkspaceView({
  id: "terminal",
  title: "workspace.view.title.terminal",
  icon: "terminal",
  order: 60,
  splittable: true,
  component: TerminalWorkspaceSurface,
});
