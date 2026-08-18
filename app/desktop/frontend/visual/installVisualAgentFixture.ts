import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import { navigator } from "@/lib/navigation";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { reduceAgentEvent } from "@/plugins/builtin/agent/application/fold/reducer";
import {
  configureAgentRuntimeGateway,
  type AgentRuntimeGateway,
} from "@/plugins/builtin/agent/application/ports/runtimeGateway";
import { projectAgentSessionSnapshot } from "@/plugins/builtin/agent/application/session/sessionSnapshot";
import agentFold from "@/plugins/builtin/agent/bootstrap/foldPlugin";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { APPROVAL_MODE_KEY } from "@/plugins/builtin/agent/public/approvalPolicy";
import { composerBootstrap, composerSend, composerToolbar } from "@/plugins/builtin/chat/composer";
import contextUsage from "@/plugins/builtin/chat/context-usage";
import narrativeRails from "@/plugins/builtin/chat/narrative-rails";
import {
  messageCopy,
  messageEdit,
  messageFeedback,
  messageRegenerate,
} from "@/plugins/builtin/chat/message-actions";
import { builtinVisualStyles } from "@/plugins/builtin/theme/visualStyles";
import lyraDark from "@/plugins/builtin/theme/themes/lyra-dark";
import lyraLight from "@/plugins/builtin/theme/themes/lyra-light";
import { defaultAccents } from "@/plugins/builtin/defaults";
import goal from "@/plugins/builtin/chat/goal";
import type { GoalState } from "@/plugins/builtin/chat/goal/application/goalReadModel";
import planProgress from "@/plugins/builtin/chat/plan-progress";
import {
  MODELS_KEY,
  type SelectableModel,
} from "@/plugins/builtin/settings/providers/public/queries";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";
import { installRuntimeCapabilityPort } from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import {
  configureRuntimeServiceStatusPort,
  type RuntimeServiceSnapshot,
} from "@/plugins/builtin/runtime/application/ports/serviceStatus";
import { queryClient } from "@/lib/queryClient";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { toolRenderingPlugins } from "@/plugins/builtin";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import {
  WORKSPACE_FILE_HEAD_KEY,
  type WorkspaceFileLine,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useComposerStore } from "@/plugins/builtin/chat/composer/adapters/composerStore";
import {
  AGENT_SESSION_SNAPSHOTS,
  AGENT_SESSION_TAIL_EVENTS,
  VISUAL_GOALS,
  VISUAL_ROOT_RUN_ID,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";

const VISUAL_MODELS: SelectableModel[] = [
  {
    id: "gpt-5.6",
    provider: "openai",
    label: "GPT-5.6",
    multimodal: true,
    contextWindow: 256_000,
  },
];

function visualSession(state: VisualAgentState): AgentSessionSummary {
  return {
    id: VISUAL_SESSION_ID,
    revision: 7,
    title: `Agent · ${state}`,
    status:
      state === "waiting" || state === "question"
        ? "waiting"
        : state === "running" || state === "steer" || state === "delegated"
          ? "running"
          : "idle",
    model: VISUAL_MODELS[0]!.id,
    cwd: "/Users/visual/lynx",
    time: "2026-07-31T08:00:00.000Z",
  };
}

function visualAgentRuntimeGateway(state: VisualAgentState): AgentRuntimeGateway {
  const snapshot = AGENT_SESSION_SNAPSHOTS[state];
  return {
    createSession: async () => ({ id: VISUAL_SESSION_ID }),
    deleteSession: async () => undefined,
    updateSession: async ({ expectedRevision }) => ({ revision: expectedRevision + 1 }),
    forkSession: async () => ({ id: `${VISUAL_SESSION_ID}_fork` }),
    loadSessionSnapshot: async () => ({
      snapshot,
      projectAssociatedSharedMaterial: (shared) => shared,
    }),
    loadSessionUsage: async () => ({}),
    rollbackSession: async () => ({ droppedRuns: [] }),
    steerRun: async (runId, segmentId, input) => {
      document.documentElement.dataset.visualSteeredRun = runId;
      document.documentElement.dataset.visualSteeredSegment = segmentId;
      document.documentElement.dataset.visualSentInput = JSON.stringify(input);
    },
    isRunGone: () => false,
    isReplayLost: () => false,
    setApprovalMode: async (mode) => mode,
    forgetApprovalRule: async () => undefined,
  };
}

// The read preview asks the workspace for a file's head, which is a data provider and
// not part of the tool result — so without one the preview rendered empty here and the
// component that draws it appeared in no test. One line is deliberately far longer than
// the column it is read in, which is where it used to lose its tail.
const fileHeadProvider = definePlugin({
  name: "lyra.visual.file-head",
  setup(ctx) {
    ctx.contribute(DATA_PROVIDER, {
      key: WORKSPACE_FILE_HEAD_KEY,
      fetcher: async () =>
        [
          { lineNumber: 1, text: "package session" },
          { lineNumber: 2, text: "" },
          {
            lineNumber: 3,
            text: "func (s *Store) Commit(ctx context.Context, session *Session, records []Record, opts CommitOptions) (Receipt, error) {",
          },
          { lineNumber: 4, text: "\tif err := s.flushLocked(); err != nil {" },
          { lineNumber: 5, text: '\t\treturn Receipt{}, fmt.Errorf("commit: %w", err)' },
          { lineNumber: 6, text: "\t}" },
        ] satisfies WorkspaceFileLine[],
    });
  },
});

/**
 * A connected Runtime, stated rather than probed.
 *
 * The transcript asks whether commands may be sent (RunErrorBanner gates Retry on
 * it), and in production the answer comes from a live health check. A fixture that
 * left the port unconfigured did not render a degraded banner — it threw out of
 * the first component to ask, so every screenshot in four spec files was of an
 * error boundary. Deterministic by construction: one frozen observation, and
 * refresh does nothing, because a golden must not depend on a probe.
 */
function installVisualRuntimeServiceStatusPort(): void {
  const snapshot = {
    phase: "ready",
    observation: {
      server: { name: "lyra-runtime", version: "0.0.0-visual" },
      protocol: { current: "2", minSupported: "2" },
      health: "ready",
      checks: {},
    },
    failure: null,
  } as const satisfies RuntimeServiceSnapshot;

  configureRuntimeServiceStatusPort({
    useSnapshot: () => snapshot,
    snapshot: () => snapshot,
    refresh: () => Promise.resolve(),
  });
}

export async function installVisualAgentFixture(
  state: VisualAgentState,
): Promise<AgentSessionView> {
  queryClient.clear();
  installRuntimeCapabilityPort();
  installVisualRuntimeServiceStatusPort();
  installAgentStatePorts();
  installWorkspaceNavigationPort();
  configureAgentRuntimeGateway(visualAgentRuntimeGateway(state));

  useAgentSessionStore.setState({
    openSessionIds: [VISUAL_SESSION_ID],
    lastSessionId: VISUAL_SESSION_ID,
    draftSessionIds: new Set(),
    pendingMessages: {},
  });
  // Which session is on screen is the location, not a store field.
  navigator().go({ session: VISUAL_SESSION_ID });
  queryClient.setQueryDefaults([AGENT_SESSIONS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([MODELS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([APPROVAL_MODE_KEY], { staleTime: Infinity });
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [visualSession(state)]);
  queryClient.setQueryData([MODELS_KEY], VISUAL_MODELS);
  queryClient.setQueryData([APPROVAL_MODE_KEY], "ask");
  await loadPluginsForTest(
    // The palettes and the geometry, or the fixture photographs globals.css's
    // pre-hydration fallbacks: an unregistered theme id resolves to the dark
    // scheme, which is why every `agent-light-*` golden was a byte-for-byte copy
    // of its dark twin.
    lyraLight,
    defaultAccents,
    lyraDark,
    ...builtinVisualStyles,
    agentFold,
    composerBootstrap,
    composerToolbar,
    contextUsage,
    composerSend,
    narrativeRails,
    // The per-message action bar. Unregistered, the slot rendered nothing, so
    // every agent golden framed a transcript with no controls on it — which is
    // how the bar spent its life in the caption line, running off the far edge
    // of the reading column, without a single screenshot showing it.
    messageCopy,
    messageEdit,
    messageRegenerate,
    messageFeedback,
    goal,
    planProgress,
    // Production's own tool-rendering list, not a hand-picked subset of it: the
    // subset held four of fifteen, so the previews for edit, read and grep — all of
    // which the canonical snapshots carry results for — rendered as raw JSON here
    // while the app rendered the real component.
    ...toolRenderingPlugins,
    fileHeadProvider,
  );

  // composerBootstrap synchronizes the active session draft while it loads;
  // install the fixture draft after that production bootstrap has completed.
  useComposerStore.setState({
    value: state === "steer" ? "Tighten the error copy and continue." : "",
    images: [],
    pastes: [],
    provider: VISUAL_MODELS[0]!.provider,
    model: VISUAL_MODELS[0]!.id,
  });

  let view = projectAgentSessionSnapshot(AGENT_SESSION_SNAPSHOTS[state]);
  for (const event of AGENT_SESSION_TAIL_EVENTS[state]) {
    view = reduceAgentEvent(view, event);
  }
  view = {
    ...view,
    shared: {
      ...view.shared,
      goal: {
        available: true,
        goal: VISUAL_GOALS[state] ?? null,
      } satisfies GoalState,
    },
  };

  const store = useAgentStore.getState();
  store.ensureSession(VISUAL_SESSION_ID);
  const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
  if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, view)) {
    throw new Error(`Failed to install visual agent state "${state}"`);
  }
  store.setSend(VISUAL_SESSION_ID, (input) => {
    document.documentElement.dataset.visualSentInput = JSON.stringify(input);
    return true;
  });
  store.setStop(VISUAL_SESSION_ID, () => {
    document.documentElement.dataset.visualStoppedRoot = VISUAL_ROOT_RUN_ID;
    return true;
  });
  store.setResume(VISUAL_SESSION_ID, (runId, responses, onSettled) => {
    document.documentElement.dataset.visualResumedRun = runId;
    document.documentElement.dataset.visualResumedItem = responses[0]?.itemId ?? "";
    onSettled?.();
    return true;
  });
  store.setCancelRun(VISUAL_SESSION_ID, (runId) => {
    document.documentElement.dataset.visualCanceledRun = runId;
  });

  return view;
}
