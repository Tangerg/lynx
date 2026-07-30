// Regression: useAgentProblem / useAgentSharedState must react to an
// activeSessionId switch, not just to agent-store mutations. They read the
// active session's view, and activeSessionId lives in a SEPARATE store
// (useAgentSessionStore); if the switch isn't a reactive dependency, a
// consumer keeps rendering the previous session's error / shared state until
// the agent store happens to mutate. Locking the reactive contract here keeps
// these two selectors from drifting off the useActiveAgentView pattern the
// other view selectors share.

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW, type AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { useAgentStore } from "./agentStore";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentProblem, useAgentSharedState } from "./agentViewSelectors";

function seed(commandError: AgentProblem | null, shared: Record<string, unknown>) {
  return {
    view: { ...EMPTY_AGENT_SESSION_VIEW, commandError, shared },
    viewEpoch: 0,
    stop: null,
    send: null,
    resume: null,
  };
}

afterEach(() => {
  useAgentStore.setState({ sessions: {} });
  useAgentSessionStore.setState({ activeSessionId: "" });
});

describe("agent view selectors react to session switch", () => {
  it("useAgentProblem follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed({ message: "A" }, {}), b: seed({ message: "B" }, {}) },
    });
    useAgentSessionStore.setState({ activeSessionId: "a" });

    const { result } = renderHook(() => useAgentProblem());
    expect(result.current?.message).toBe("A");

    act(() => useAgentSessionStore.setState({ activeSessionId: "b" }));
    expect(result.current?.message).toBe("B");
  });

  it("useAgentSharedState follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed(null, { k: "A" }), b: seed(null, { k: "B" }) },
    });
    useAgentSessionStore.setState({ activeSessionId: "a" });

    const { result } = renderHook(() => useAgentSharedState<string>("k"));
    expect(result.current).toBe("A");

    act(() => useAgentSessionStore.setState({ activeSessionId: "b" }));
    expect(result.current).toBe("B");
  });
});
