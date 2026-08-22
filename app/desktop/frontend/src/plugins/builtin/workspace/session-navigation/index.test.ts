import { beforeEach, describe, expect, it, vi } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import sessionNavigation from ".";
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { WORKSPACE_SCOPE_PORTS } from "@/plugins/builtin/workspace/public/ports";
import {
  activateWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

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

// The plugin declares its ports, so the fake is installed as their provider
// rather than mocked into a module path it no longer imports.
const ports = definePlugin({
  name: "test.session-ports",
  provides: { sessions: AGENT_SESSION_PORTS, scopes: WORKSPACE_SCOPE_PORTS },
  setup: () => ({
    sessions: {
      activeSessionId: () => navigator().get().session,
      lifecycleSnapshot: () => ({
        activeSessionId: navigator().get().session,
        openSessionIds: agentSession.getOpen(),
      }),
      subscribeActiveSessionId: (fn: (sessionId: string) => void) => agentSession.subscribe(fn),
      subscribeLifecycle: () => () => {},
    },
    scopes: {
      activateSessionScope: activateWorkspaceSessionScope,
      forgetSessionScopes: forgetWorkspaceSessionScopes,
    },
  }),
});

beforeEach(() => {
  agentSession.reset();
  navigator().go({ session: "s1" });
});

describe("workspace session navigation", () => {
  it("adopts the session the app is already in when it loads", async () => {
    useContextDockStore.setState({ activeSessionScopeId: null, sessionScopes: new Map() });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });

  it("adopts the dock location that survived a renderer replacement", async () => {
    navigator().go({ session: "s1", dock: "diff" });
    useContextDockStore.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
      dockViewIds: [],
      lastViewId: null,
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("restores the dock destination the session it moves to remembers", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    useContextDockStore.getState().openDockTab("diff");
    useContextDockStore.getState().rememberDockView("diff");

    agentSession.goTo("s2");
    expect(navigator().get().dock).toBeNull();

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
    expect(navigator().get().dock).toBe("diff");
  });

  it("keeps each session's own tabs", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    useContextDockStore.getState().openDockTab("diff");

    agentSession.goTo("s2");
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("stops following once unloaded", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    await resetKernelForTest();

    agentSession.goTo("s2");

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });
});
