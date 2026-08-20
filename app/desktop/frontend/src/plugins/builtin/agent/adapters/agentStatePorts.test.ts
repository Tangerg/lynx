import { beforeEach, describe, expect, it } from "vitest";
import { navigator } from "@/lib/navigation";
import { agentSessionState } from "../application/ports/sessionState";
import { useAgentSessionStore } from "./agentSessionStore";

beforeEach(() => {
  navigator().go({ session: "", view: null, dock: null, settings: null });
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
    draftSessionIds: new Set<string>(),
    freshDraftSessionIds: new Set<string>(),
  });
});

describe("Agent session state adapter continuity", () => {
  it("remembers the adjacent survivor after the active Session closes", () => {
    navigator().go({ session: "ses_a", view: "diff", dock: "terminal" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_a", "ses_b"],
      lastSessionId: "ses_a",
    });

    agentSessionState().closeSession("ses_a");

    expect(navigator().get().session).toBe("ses_b");
    expect(navigator().get().view).toBeNull();
    expect(navigator().get().dock).toBe("terminal");
    expect(useAgentSessionStore.getState().lastSessionId).toBe("ses_b");
  });

  it("retires a deleted deep-link's promoted view with the authoritative Session", () => {
    navigator().go({ session: "ses_gone", view: "run-summary", dock: "diff" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_live", "ses_gone"],
      lastSessionId: "ses_gone",
    });

    agentSessionState().reconcileSessions(["ses_live"]);

    expect(navigator().get()).toMatchObject({
      session: "ses_live",
      view: null,
      dock: "diff",
    });
    expect(useAgentSessionStore.getState().lastSessionId).toBe("ses_live");
  });

  it("keeps the active Session's promoted view when a background Session closes", () => {
    navigator().go({ session: "ses_a", view: "diff", dock: "terminal" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_a", "ses_b"],
      lastSessionId: "ses_a",
    });

    agentSessionState().closeSession("ses_b");

    expect(navigator().get()).toMatchObject({
      session: "ses_a",
      view: "diff",
      dock: "terminal",
    });
    expect(useAgentSessionStore.getState().lastSessionId).toBe("ses_a");
  });

  it("keeps the active Session's promoted view when reconciliation only trims its peers", () => {
    navigator().go({ session: "ses_a", view: "run-summary", dock: "diff" });
    useAgentSessionStore.setState({
      openSessionIds: ["ses_a", "ses_gone"],
      lastSessionId: "ses_a",
    });

    agentSessionState().reconcileSessions(["ses_a"]);

    expect(navigator().get()).toMatchObject({
      session: "ses_a",
      view: "run-summary",
      dock: "diff",
    });
    expect(useAgentSessionStore.getState().lastSessionId).toBe("ses_a");
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
