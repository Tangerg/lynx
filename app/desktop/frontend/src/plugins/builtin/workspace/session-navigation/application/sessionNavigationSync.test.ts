import { describe, expect, it, vi } from "vitest";
import type {
  AgentSessionLifecycleSnapshot,
  AgentSessionSelectionSnapshot,
} from "@/plugins/builtin/agent/public/session";
import {
  bindWorkspaceSessionNavigation,
  syncWorkspaceSessionLifecycle,
  syncWorkspaceSessionSelection,
  type AgentSessionLifecycleListener,
  type AgentSessionSelectionListener,
  type WorkspaceSessionNavigationPorts,
} from "./sessionNavigationSync";

const selection = (
  activeSessionId: string,
  selectionEpoch: number,
): AgentSessionSelectionSnapshot => ({ activeSessionId, selectionEpoch });

const lifecycle = (openSessionIds: string[]): AgentSessionLifecycleSnapshot => ({
  activeSessionId: openSessionIds[0] ?? "",
  openSessionIds,
});

describe("syncWorkspaceSessionSelection", () => {
  it("returns the workspace to chat when a user selects a session", () => {
    const selectChat = vi.fn();
    const activateSessionScope = vi.fn();

    syncWorkspaceSessionSelection(selection("s1", 2), selection("s1", 1), {
      selectChat,
      activateSessionScope,
    });

    expect(selectChat).toHaveBeenCalledOnce();
    expect(activateSessionScope).not.toHaveBeenCalled();
  });

  it("activates a new workspace scope when the active session changes", () => {
    const selectChat = vi.fn();
    const activateSessionScope = vi.fn();

    syncWorkspaceSessionSelection(selection("s2", 1), selection("s1", 1), {
      selectChat,
      activateSessionScope,
    });

    expect(selectChat).not.toHaveBeenCalled();
    expect(activateSessionScope).toHaveBeenCalledWith("s2");
  });
});

describe("syncWorkspaceSessionLifecycle", () => {
  it("forgets workspace scopes outside the open session set", () => {
    const forgetSessionScopes = vi.fn();

    syncWorkspaceSessionLifecycle(lifecycle(["s2"]), { forgetSessionScopes });

    expect(forgetSessionScopes).toHaveBeenCalledWith(["s2"]);
  });
});

describe("bindWorkspaceSessionNavigation", () => {
  it("seeds the active scope, prunes closed scopes, and disposes subscriptions", () => {
    const unsubscribeSelection = vi.fn();
    const unsubscribeLifecycle = vi.fn();
    const ports: WorkspaceSessionNavigationPorts = {
      activeSessionId: () => "s1",
      lifecycleSnapshot: () => lifecycle(["s1", "s2"]),
      subscribeSelection: vi.fn((_listener: AgentSessionSelectionListener) => unsubscribeSelection),
      subscribeLifecycle: vi.fn((_listener: AgentSessionLifecycleListener) => unsubscribeLifecycle),
      activateSessionScope: vi.fn(),
      forgetSessionScopes: vi.fn(),
      selectChat: vi.fn(),
    };

    const dispose = bindWorkspaceSessionNavigation(ports);
    dispose();

    expect(ports.activateSessionScope).toHaveBeenCalledWith("s1");
    expect(ports.forgetSessionScopes).toHaveBeenCalledWith(["s1", "s2"]);
    expect(ports.subscribeSelection).toHaveBeenCalledOnce();
    expect(ports.subscribeLifecycle).toHaveBeenCalledOnce();
    expect(unsubscribeSelection).toHaveBeenCalledOnce();
    expect(unsubscribeLifecycle).toHaveBeenCalledOnce();
  });
});
