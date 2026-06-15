import { useMemo } from "react";
import { EmptyState } from "@/components/common";
import { toolCategory } from "@/protocol/run/viewState";
import { useAgentSlice } from "@/state/agentStore";
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
  const toolCalls = useAgentSlice((v) => v.toolCalls);
  const commands = useMemo(
    () => Object.values(toolCalls).filter((t) => toolCategory(t.name) === "command"),
    [toolCalls],
  );

  return (
    <WorkspaceViewLayout
      icon="terminal"
      title="Terminal"
      sub={commands.length ? `${commands.length} commands` : undefined}
    >
      {commands.length === 0 ? (
        <EmptyState
          icon="terminal"
          title="No commands yet"
          sub="Commands the agent runs show up here with their output."
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
