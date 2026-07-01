import { useEffect, useMemo, useRef } from "react";
import { EmptyState } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useActiveRunToolCalls } from "@/plugins/builtin/agent/public/run";
import { workspaceCommandToolsFromAgentTools } from "../application/toolActivity";
import { CommandLog } from "./views/CommandLog";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

// The agent's command log (G5). Each command's output streams via
// item.delta{toolOutput} → item.completed — 613 confirmed that's already on
// the wire (no new API), and the run fold lands it in view.toolCalls. This view
// just consolidates the command-category tools into one terminal-like surface.
// (A user-interactive PTY is deliberately out of the runtime's scope, so this is
// a read-only log of what the agent ran, not an input terminal.)
function TerminalTab() {
  const t = useT();
  const toolCalls = useActiveRunToolCalls();
  const commands = useMemo(() => workspaceCommandToolsFromAgentTools(toolCalls), [toolCalls]);

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
  // Cheap content signature — grows as commands are added or their output
  // streams; re-pins to the bottom on each change while the user is pinned.
  const tail = commands.reduce((n, c) => n + (c.result?.length ?? 0), commands.length);
  useEffect(() => {
    if (!pinnedRef.current) return;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [tail]);

  return (
    <WorkspaceViewLayout
      icon="terminal"
      title="terminal.title"
      sub={commands.length ? `${commands.length} commands` : undefined}
      scrollRef={scrollRef}
    >
      {commands.length === 0 ? (
        <EmptyState
          icon="terminal"
          title={t("terminal.empty.title")}
          sub={t("terminal.empty.sub")}
        />
      ) : (
        <CommandLog commands={commands} />
      )}
    </WorkspaceViewLayout>
  );
}

export const terminalView = defineWorkspaceView({
  id: "terminal",
  title: "workspace.view.title.terminal",
  icon: "terminal",
  openByDefault: false,
  order: 10,
  splittable: true,
  component: TerminalTab,
});
