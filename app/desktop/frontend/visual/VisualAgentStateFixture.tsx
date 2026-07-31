import {
  selectCurrentRootAttention,
  selectCurrentRootPlan,
  selectDelegatedRunNarratives,
  selectRootNarrativeMessages,
  selectRunTree,
  selectVisibleProblem,
} from "@/plugins/builtin/agent/application/view/runTree";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { MessageBlock, type BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import {
  AgentAppShell,
  AgentComposerSurface,
  AgentContentCard,
  AgentRow,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";
import { Button, Icon, IconButton } from "@/ui";
import type { VisualAgentState } from "./agentSessionSnapshots";

const STATE_LABELS: Record<VisualAgentState, string> = {
  empty: "Empty",
  idle: "Idle",
  running: "Running",
  waiting: "Waiting",
  terminal: "Terminal",
  error: "Error",
  delegated: "Delegated tree",
  "long-content": "Long content",
};

function StateSidebar({ state }: { state: VisualAgentState }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-lg font-semibold text-fg">Agent states</span>
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

function ErrorFixture({ code, message }: { code: string; message?: string }) {
  return (
    <div role="alert" className="mb-4 rounded-lg bg-negative-wash px-4 py-3 text-fg">
      <div className="flex items-center gap-2 text-ui-lg font-semibold text-negative">
        <Icon name="bug" size={14} />
        Run failed · {code}
      </div>
      {message && <div className="mt-1 text-ui-lg leading-body text-fg-soft">{message}</div>}
      <div className="mt-2 flex gap-1.5">
        <Button variant="outline" size="xs">
          Open timeline
        </Button>
        <Button variant="outline" size="xs">
          Diagnostics
        </Button>
      </div>
    </div>
  );
}

function ComposerPreview() {
  return (
    <AgentComposerSurface data-testid="agent-state-composer">
      <div className="min-h-16 px-[var(--density-composer-editor-start)] pt-[var(--density-composer-editor-top)] pb-[var(--density-composer-editor-bottom)] text-ui-lg leading-relaxed text-fg-faint">
        Ask a follow-up…
      </div>
      <div className="flex items-center gap-1 pr-[var(--density-composer-footer-end)] pb-[var(--density-composer-footer)] pl-[var(--density-composer-footer)]">
        <Button variant="ghost" size="xs">
          Agent
        </Button>
        <span className="min-w-2 flex-1" />
        <IconButton icon="arrow-up" size="md" aria-label="Send" className="bg-cta text-cta-text" />
      </div>
    </AgentComposerSurface>
  );
}

export function VisualAgentStateFixture({
  state,
  theme,
  view,
}: {
  state: VisualAgentState;
  theme: "light" | "dark";
  view: AgentSessionView;
}) {
  const attention = selectCurrentRootAttention(view);
  const messages = selectRootNarrativeMessages(view);
  const delegatedRunsByItemId = selectDelegatedRunNarratives(view);
  const runTree = selectRunTree(view);
  const problem = selectVisibleProblem(view);
  const expandedIds = new Set(Object.keys(delegatedRunsByItemId));
  const ctx: BlockCtx = {
    plan: selectCurrentRootPlan(view),
    toolCalls: view.toolCalls,
    delegatedRunsByItemId,
    expandedIds,
    onSelectTool: () => undefined,
    onToggleExpand: () => undefined,
    typewriter: false,
  };
  const running = attention.status === "running";

  return (
    <AgentAppShell
      sidebarLabel="Agent fixture states"
      sidebarOpen
      sidebarWidth={256}
      onResize={() => undefined}
      sidebar={<StateSidebar state={state} />}
      main={
        <AgentContentCard label={`${STATE_LABELS[state]} agent state`}>
          <AgentSurfaceHeader>
            <span className="font-mono text-ui-sm text-fg-faint">fixture</span>
            <span className="text-ui-lg text-fg-faint">/</span>
            <span className="text-ui-lg font-semibold text-fg">{STATE_LABELS[state]}</span>
            <AgentStatusPill
              tone={
                attention.status === "running"
                  ? "running"
                  : attention.status === "waiting"
                    ? "warning"
                    : problem
                      ? "warning"
                      : "neutral"
              }
            >
              {attention.status}
            </AgentStatusPill>
            <span className="min-w-2 flex-1" />
            <span className="font-mono text-ui-xs text-fg-faint tabular-nums">
              {runTree.length} root · {messages.length} messages · {theme}
            </span>
          </AgentSurfaceHeader>
          <div
            className="panel-scroll flex min-h-0 flex-1 flex-col"
            data-testid="agent-state"
            data-state={state}
            data-attention={attention.status}
            data-root-count={runTree.length}
          >
            <div className="mx-auto flex w-full max-w-[var(--content-max)] flex-1 flex-col px-[var(--density-column-gutter)] pt-7 sm:px-[var(--density-column-gutter-wide)]">
              {problem && <ErrorFixture code={problem.code} message={problem.message} />}
              {messages.length === 0 ? (
                <div className="flex flex-1 flex-col items-center justify-center pb-10 text-center">
                  <h1 className="text-display-lg font-normal text-fg">What should we build?</h1>
                  <p className="mt-2 text-ui-lg text-fg-muted">
                    This state contains no invented demo Run or Item.
                  </p>
                </div>
              ) : (
                <div className="flex flex-col gap-8 pb-8">
                  {messages.map((message, index) => (
                    <MessageBlock
                      key={message.id}
                      msg={message}
                      ctx={ctx}
                      isLast={index === messages.length - 1}
                      isRunning={running}
                    />
                  ))}
                </div>
              )}
              <div className="min-h-4 flex-1" />
              <ComposerPreview />
              <div className="h-4 shrink-0" />
            </div>
          </div>
        </AgentContentCard>
      }
    />
  );
}
