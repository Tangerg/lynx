import {
  selectCurrentRootAttention,
  selectRootNarrativeMessages,
  selectRunTree,
  selectVisibleProblem,
} from "@/plugins/builtin/agent/application/view/runTree";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualAgentState } from "./agentSessionSnapshots";

const STATE_LABELS: Record<VisualAgentState, string> = {
  empty: "Empty",
  idle: "Idle",
  running: "Running",
  steer: "Steer",
  waiting: "Waiting",
  question: "Question",
  terminal: "Terminal",
  canceled: "Canceled",
  error: "Error",
  recovery: "Recovery",
  delegated: "Delegated tree",
  "long-content": "Long content",
  narrative: "Narrative",
};

function StateSidebar({ state }: { state: VisualAgentState }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-md font-semibold text-fg">Agent states</span>
      </AgentSurfaceHeader>
      <div className="flex flex-col gap-0.5 px-2 pt-2">
        {(Object.keys(STATE_LABELS) as VisualAgentState[]).map((candidate) => (
          <AgentRow
            key={candidate}
            icon={candidate === "error" ? "alert" : candidate === "delegated" ? "bot" : "chat"}
            active={candidate === state}
          >
            {STATE_LABELS[candidate]}
          </AgentRow>
        ))}
      </div>
      <div className="min-h-4 flex-1" />
      <div className="px-4 pb-3 text-ui-xs leading-body text-fg-faint">
        Canonical snapshot → production projection
      </div>
    </div>
  );
}

export function VisualAgentStateFixture({
  state,
  view,
}: {
  state: VisualAgentState;
  view: AgentSessionView;
}) {
  const attention = selectCurrentRootAttention(view);
  const messages = selectRootNarrativeMessages(view);
  const runTree = selectRunTree(view);
  const problem = selectVisibleProblem(view);

  return (
    <AgentAppShell
      sidebarLabel="Agent fixture states"
      sidebarResizeLabel="Resize the agent fixture sidebar"
      sidebarOpen
      sidebarWidth={256}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the agent fixture sidebar"
      sidebarCollapseLabel="Collapse the agent fixture sidebar"
      sidebar={<StateSidebar state={state} />}
      main={
        <div
          className="contents"
          data-testid="agent-state"
          data-state={state}
          data-attention={attention.status}
          data-root-count={runTree.length}
          data-message-count={messages.length}
          data-problem={problem?.code}
        >
          <ChatPanel onSend={() => undefined} />
        </div>
      }
    />
  );
}
