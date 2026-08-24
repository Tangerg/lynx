import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { queryClient } from "@/lib/queryClient";
import { configureAgentRuntimeGateway, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { configureAgentSessionStatePort, type AgentSessionStatePort } from "../ports/sessionState";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "./sessionQueries";
import { useDeleteSession } from "./deleteSession";
import { AgentCommandOwner } from "../agentCommandOwner";

let restoreRuntime: (() => void) | undefined;
let restoreState: (() => void) | undefined;

function session(id: string): AgentSessionSummary {
  return {
    id,
    revision: 1,
    title: id,
    status: "idle",
    provider: "openai",
    model: "gpt-5",
    workspace: { path: "/repo", availability: "available" },
    time: "2026-08-12T00:00:00Z",
  };
}

afterEach(() => {
  restoreRuntime?.();
  restoreState?.();
  restoreRuntime = undefined;
  restoreState = undefined;
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("useDeleteSession", () => {
  it("keeps Session identity authoritative until the delete command commits", async () => {
    let commit!: () => void;
    const deleteSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          commit = resolve;
        }),
    );
    const closeSession = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      deleteSession,
    } as unknown as AgentRuntimeGateway);
    restoreState = configureAgentSessionStatePort({
      closeSession,
    } as unknown as AgentSessionStatePort);
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session("ses_a"), session("ses_b")]);
    const { result } = renderHook(() => useDeleteSession());

    let deleting!: Promise<void>;
    await act(async () => {
      deleting = result.current("ses_a");
      await Promise.resolve();
    });

    expect(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])).toEqual([
      session("ses_a"),
      session("ses_b"),
    ]);
    expect(closeSession).not.toHaveBeenCalled();

    commit();
    await act(async () => deleting);
    expect(closeSession).toHaveBeenCalledWith("ses_a");
  });

  it("retires delete loading before an old non-cooperative RPC responds", async () => {
    let commit!: () => void;
    const deleteSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          commit = resolve;
        }),
    );
    const closeSession = vi.fn();
    restoreRuntime = configureAgentRuntimeGateway({
      deleteSession,
    } as unknown as AgentRuntimeGateway);
    restoreState = configureAgentSessionStatePort({
      closeSession,
    } as unknown as AgentSessionStatePort);
    const retiredOwner = AgentCommandOwner.install();
    const { result } = renderHook(() => useDeleteSession());
    const deleting = result.current("ses_a");
    await Promise.resolve();
    expect(deleteSession).toHaveBeenCalledOnce();

    const successor = AgentCommandOwner.install();
    let settled = false;
    void deleting.then(() => {
      settled = true;
    });
    await Promise.resolve();
    await Promise.resolve();
    const settledBeforeOldRPC = settled;

    commit();
    await deleting;
    expect(settledBeforeOldRPC).toBe(true);
    expect(closeSession).not.toHaveBeenCalled();
    retiredOwner.dispose();
    successor.dispose();
  });
});
