import { describe, expect, it, vi } from "vitest";
import {
  bindWorkspaceSessionNavigation,
  syncWorkspaceSessionLifecycle,
  type WorkspaceSessionNavigationPorts,
} from "./sessionNavigationSync";

function ports(overrides: Partial<WorkspaceSessionNavigationPorts> = {}) {
  return {
    activeSessionId: vi.fn(() => "s1"),
    lifecycleSnapshot: vi.fn(() => ({ activeSessionId: "s1", openSessionIds: ["s1"] })),
    subscribeActiveSessionId: vi.fn(() => () => {}),
    subscribeLifecycle: vi.fn(() => () => {}),
    activateSessionScope: vi.fn(),
    forgetSessionScopes: vi.fn(),
    ...overrides,
  } satisfies WorkspaceSessionNavigationPorts;
}

describe("syncWorkspaceSessionLifecycle", () => {
  it("forgets the scopes of sessions no longer open", () => {
    const p = ports();
    syncWorkspaceSessionLifecycle({ activeSessionId: "s1", openSessionIds: ["s1"] }, p);
    expect(p.forgetSessionScopes).toHaveBeenCalledWith(["s1"]);
  });
});

describe("bindWorkspaceSessionNavigation", () => {
  it("adopts the session the app is already in", () => {
    const p = ports();
    bindWorkspaceSessionNavigation(p);

    expect(p.activateSessionScope).toHaveBeenCalledWith("s1");
    expect(p.forgetSessionScopes).toHaveBeenCalledWith(["s1"]);
  });

  it("follows the session the user moves to", () => {
    let listener: ((sessionId: string) => void) | undefined;
    const p = ports({
      subscribeActiveSessionId: vi.fn((fn: (sessionId: string) => void) => {
        listener = fn;
        return () => {};
      }),
    });
    bindWorkspaceSessionNavigation(p);

    listener?.("s2");

    expect(p.activateSessionScope).toHaveBeenLastCalledWith("s2");
  });

  it("unsubscribes both feeds when disposed", () => {
    const unsubscribeSession = vi.fn();
    const unsubscribeLifecycle = vi.fn();
    const dispose = bindWorkspaceSessionNavigation(
      ports({
        subscribeActiveSessionId: vi.fn(() => unsubscribeSession),
        subscribeLifecycle: vi.fn(() => unsubscribeLifecycle),
      }),
    );

    dispose();

    expect(unsubscribeSession).toHaveBeenCalledOnce();
    expect(unsubscribeLifecycle).toHaveBeenCalledOnce();
  });
});
