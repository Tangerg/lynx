import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import { navigator } from "@/lib/navigation";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { reduceRunEvent } from "@/plugins/builtin/agent/application/fold/reducer";
import {
  configureAgentRuntimeGateway,
  type AgentRuntimeGateway,
} from "@/plugins/builtin/agent/application/ports/runtimeGateway";
import { projectAgentSessionSnapshot } from "@/plugins/builtin/agent/application/session/sessionSnapshot";
import agentFold from "@/plugins/builtin/agent/public/foldPlugin";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { APPROVAL_MODE_KEY } from "@/plugins/builtin/agent/public/approvalPolicy";
import { composerBootstrap, composerSend, composerToolbar } from "@/plugins/builtin/chat/composer";
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
import planProgress from "@/plugins/builtin/chat/plan-progress";
import { toolActions, toolIcons } from "@/plugins/builtin/chat/tools/meta";
import { shellPreview, taskPreview } from "@/plugins/builtin/chat/tools/previews";
import {
  MODELS_KEY,
  type SelectableModel,
} from "@/plugins/builtin/settings/providers/public/queries";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";
import { installRuntimeCapabilityPort } from "@/plugins/builtin/runtime/adapters/runtimeCapabilityStore";
import { queryClient } from "@/lib/queryClient";
import { loadPlugin, usePluginStore } from "@/plugins/sdk";
import { useComposerStore } from "@/plugins/builtin/chat/composer/adapters/composerStore";
import {
  AGENT_SESSION_SNAPSHOTS,
  AGENT_SESSION_TAIL_EVENTS,
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
    loadSessionSnapshot: async () => snapshot,
    sessionHoldsNothing: async () => snapshot.items.length === 0,
    loadSessionUsage: async () => ({}),
    rollbackSession: async () => undefined,
    steerRun: async (runId, segmentId, input) => {
      document.documentElement.dataset.visualSteeredRun = runId;
      document.documentElement.dataset.visualSteeredSegment = segmentId;
      document.documentElement.dataset.visualSentInput = JSON.stringify(input);
    },
    isRunGone: () => false,
    isReplayLost: () => false,
    setApprovalMode: async () => undefined,
    forgetApprovalRule: async () => undefined,
  };
}

export async function installVisualAgentFixture(
  state: VisualAgentState,
): Promise<AgentSessionView> {
  usePluginStore.getState().resetForTest();
  queryClient.clear();
  installRuntimeCapabilityPort();
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

  for (const plugin of [
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
    planProgress,
    toolIcons,
    toolActions,
    shellPreview,
    taskPreview,
  ]) {
    const result = await loadPlugin(plugin);
    if (result.kind !== "loaded") {
      throw new Error(`Failed to install visual agent plugin "${result.name}": ${result.kind}`);
    }
  }

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
    view = reduceRunEvent(view, event);
  }

  const store = useAgentStore.getState();
  store.ensureSession(VISUAL_SESSION_ID);
  const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
  if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, view)) {
    throw new Error(`Failed to install visual agent state "${state}"`);
  }
  store.setSend(VISUAL_SESSION_ID, (input) => {
    document.documentElement.dataset.visualSentInput = JSON.stringify(input);
  });
  store.setStop(VISUAL_SESSION_ID, () => {
    document.documentElement.dataset.visualStoppedRoot = VISUAL_ROOT_RUN_ID;
    return true;
  });
  store.setResume(VISUAL_SESSION_ID, (runId, responses, onSettled) => {
    document.documentElement.dataset.visualResumedRun = runId;
    document.documentElement.dataset.visualResumedItem = responses[0]?.itemId ?? "";
    onSettled?.();
  });
  store.setCancelRun(VISUAL_SESSION_ID, (runId) => {
    document.documentElement.dataset.visualCanceledRun = runId;
  });

  return view;
}
