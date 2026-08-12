import { beforeEach, describe, expect, it } from "vitest";
import { navigator } from "@/lib/navigation";
import { agentSessionState } from "../application/ports/sessionState";
import { useAgentSessionStore } from "./agentSessionStore";

beforeEach(() => {
  navigator().go({ session: "" });
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
    draftSessionIds: new Set<string>(),
    pendingMessages: {},
  });
});

describe("Agent session state adapter continuity", () => {
  it("remembers the adjacent survivor after the active Session closes", () => {
    navigator().go({ session: "ses_a" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_a", "ses_b"],
      lastSessionId: "ses_a",
    });

    agentSessionState().closeSession("ses_a");

    expect(navigator().get().session).toBe("ses_b");
    expect(useAgentSessionStore.getState().lastSessionId).toBe("ses_b");
  });

  it("forgets a missing last Session when authoritative reconciliation leaves none", () => {
    navigator().go({ session: "ses_gone" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_gone"],
      lastSessionId: "ses_gone",
    });

    agentSessionState().reconcileSessions([]);

    expect(navigator().get().session).toBe("");
    expect(useAgentSessionStore.getState().lastSessionId).toBe("");
  });
});
