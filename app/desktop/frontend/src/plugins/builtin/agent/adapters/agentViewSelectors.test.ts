// Regression: useAgentProblem / useAgentSharedState must react to an
// activeSessionId switch, not just to agent-store mutations. They read the
// active session's view, and activeSessionId lives in a SEPARATE store
// (useAgentSessionStore); if the switch isn't a reactive dependency, a
// consumer keeps rendering the previous session's error / shared state until
// the agent store happens to mutate. Locking the reactive contract here keeps
// these two selectors from drifting off the useActiveAgentView pattern the
// other view selectors share.

import { act, renderHook } from "@testing-library/react";
import { navigator } from "@/lib/navigation";
import { afterEach, describe, expect, it } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW, type AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { useAgentStore } from "./agentStore";
import {
  useAgentProblem,
  useAgentSharedState,
  useCurrentRootAttention,
  useCurrentRootOutcome,
} from "./agentViewSelectors";

function seed(commandError: AgentProblem | null, shared: Record<string, unknown>) {
  return {
    view: { ...EMPTY_AGENT_SESSION_VIEW, commandError, shared },
    viewEpoch: 0,
    viewRevision: 0,
    refreshSequence: 0,
    stop: null,
    send: null,
    resume: null,
    synchronize: null,
    cancelRun: null,
  };
}

afterEach(() => {
  useAgentStore.setState({ sessions: {} });
  navigator().go({ session: "" });
});

describe("agent view selectors react to session switch", () => {
  it("useAgentProblem follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed({ message: "A" }, {}), b: seed({ message: "B" }, {}) },
    });
    navigator().go({ session: "a" });

    const { result } = renderHook(() => useAgentProblem());
    expect(result.current?.message).toBe("A");

    act(() => navigator().go({ session: "b" }));
    expect(result.current?.message).toBe("B");
  });

  it("useAgentSharedState follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed(null, { k: "A" }), b: seed(null, { k: "B" }) },
    });
    navigator().go({ session: "a" });

    const { result } = renderHook(() => useAgentSharedState<string>("k"));
    expect(result.current).toBe("A");

    act(() => navigator().go({ session: "b" }));
    expect(result.current).toBe("B");
  });

  it("keeps the root attention snapshot referentially stable between unchanged renders", () => {
    const running = {
      id: "run-1",
      sessionId: "a",
      parentRunId: null,
      rootRunId: "run-1",
      spawnedByItemId: null,
      status: "running" as const,
      activeSegmentId: "segment-1",
      outcome: null,
      metrics: {
        steps: 0,
        activeDurationMillis: 0,
        usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
      },
      progress: null,
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: null,
    };
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [running.id]: running } },
        },
      },
    });
    navigator().go({ session: "a" });

    const { result, rerender } = renderHook(() => useCurrentRootAttention());
    const first = result.current;
    rerender();

    expect(result.current).toBe(first);
    expect(result.current).toEqual({ status: "running", runId: "run-1" });
  });

  it("does not re-render the root outcome subscriber for unrelated projection writes", () => {
    const outcome = { type: "completed" as const };
    const finished = {
      id: "run-1",
      sessionId: "a",
      parentRunId: null,
      rootRunId: "run-1",
      spawnedByItemId: null,
      status: "finished" as const,
      activeSegmentId: null,
      outcome,
      metrics: {
        steps: 1,
        activeDurationMillis: 10,
        usage: { inputTokens: 3, outputTokens: 2, cacheReadTokens: 0 },
      },
      progress: null,
      createdAt: "2026-01-01T00:00:00.000Z",
      finishedAt: "2026-01-01T00:00:01.000Z",
    };
    useAgentStore.setState({
      sessions: {
        a: {
          ...seed(null, {}),
          view: { ...EMPTY_AGENT_SESSION_VIEW, runsById: { [finished.id]: finished } },
        },
      },
    });
    navigator().go({ session: "a" });

    let renders = 0;
    const { result } = renderHook(() => {
      renders += 1;
      return useCurrentRootOutcome();
    });
    expect(result.current).toBe(outcome);

    act(() =>
      useAgentStore.setState((state) => {
        const current = state.sessions.a!;
        return {
          sessions: {
            ...state.sessions,
            a: {
              ...current,
              view: { ...current.view, shared: { unrelated: true } },
              viewRevision: current.viewRevision + 1,
            },
          },
        };
      }),
    );

    expect(renders).toBe(1);
    expect(result.current).toBe(outcome);
  });
});
