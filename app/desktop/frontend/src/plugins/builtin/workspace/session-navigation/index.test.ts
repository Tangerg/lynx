import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadPlugin, unloadPlugin } from "@/plugins/sdk";
import { useContextDockStore } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import sessionNavigation from ".";

// The plugin's job: keep the dock's per-session memory pointed at the session the
// user is in. It observes the session through the agent facade and answers by
// navigating the dock, so both sides are asserted through the location.
const agentSession = vi.hoisted(() => {
  let listener: ((sessionId: string) => void) | undefined;
  let openSessionIds = ["s1", "s2"];
  return {
    goTo(sessionId: string) {
      navigator().go({ session: sessionId });
      listener?.(sessionId);
    },
    setOpen(ids: string[]) {
      openSessionIds = ids;
    },
    getOpen: () => openSessionIds,
    subscribe(fn: (sessionId: string) => void) {
      listener = fn;
      return () => {
        listener = undefined;
      };
    },
    reset() {
      listener = undefined;
      openSessionIds = ["s1", "s2"];
    },
  };
});

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => navigator().get().session,
  getAgentSessionLifecycleSnapshot: () => ({
    activeSessionId: navigator().get().session,
    openSessionIds: agentSession.getOpen(),
  }),
  subscribeActiveSessionId: (fn: (sessionId: string) => void) => agentSession.subscribe(fn),
  subscribeAgentSessionLifecycle: () => () => {},
}));

beforeEach(() => {
  agentSession.reset();
  navigator().go({ session: "s1" });
});

afterEach(() => {
  unloadPlugin(sessionNavigation.name);
});

describe("workspace session navigation", () => {
  it("adopts the session the app is already in when it loads", async () => {
    useContextDockStore.setState({ activeSessionScopeId: "", sessionScopes: new Map() });

    await loadPlugin(sessionNavigation);

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });

  it("restores the dock destination the session it moves to remembers", async () => {
    await loadPlugin(sessionNavigation);
    useContextDockStore.getState().openDockTab("diff");
    useContextDockStore.getState().rememberDockView("diff");

    agentSession.goTo("s2");
    expect(navigator().get().dock).toBeNull();

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
    expect(navigator().get().dock).toBe("diff");
  });

  it("keeps each session's own tabs", async () => {
    await loadPlugin(sessionNavigation);
    useContextDockStore.getState().openDockTab("diff");

    agentSession.goTo("s2");
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("stops following once unloaded", async () => {
    await loadPlugin(sessionNavigation);
    await unloadPlugin(sessionNavigation.name);

    agentSession.goTo("s2");

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });
});
