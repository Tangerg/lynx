// Regression: useAgentError / useAgentSharedState must react to an
// activeSessionId switch, not just to agent-store mutations. They read the
// active session's view, and activeSessionId lives in a SEPARATE store
// (useAgentSessionStore); if the switch isn't a reactive dependency, a
// consumer keeps rendering the previous session's error / shared state until
// the agent store happens to mutate. Locking the reactive contract here keeps
// these two selectors from drifting off the useActiveAgentView pattern the
// other view selectors share.

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { INITIAL_VIEW_STATE, type RunError } from "@/plugins/builtin/agent/public/viewState";
import { useAgentStore } from "./agentStore";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentError, useAgentSharedState } from "./agentViewSelectors";

function seed(error: RunError | null, shared: Record<string, unknown>) {
  return {
    view: { ...INITIAL_VIEW_STATE, error, shared },
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
  it("useAgentError follows activeSessionId", () => {
    useAgentStore.setState({
      sessions: { a: seed({ message: "A" }, {}), b: seed({ message: "B" }, {}) },
    });
    useAgentSessionStore.setState({ activeSessionId: "a" });

    const { result } = renderHook(() => useAgentError());
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
