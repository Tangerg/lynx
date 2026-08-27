import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { configureAgentSessionStatePort, type AgentSessionStatePort } from "../ports/sessionState";
import type { AgentSessionSummary } from "./sessionQueries";
import { AGENT_SESSIONS_KEY } from "./sessionQueries";

import { useReconcilePersistedAgentSessions } from "./sessionList";

let restoreState: (() => void) | undefined;
let unmountHook: (() => void) | undefined;
let queryClient: QueryClient;

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

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [session("ses_a"), session("ses_b")]);
});

afterEach(() => {
  // Detach the QueryObserver before removing its cache entry. Removing a live
  // query transitions the observer back to pending.
  unmountHook?.();
  unmountHook = undefined;
  restoreState?.();
  restoreState = undefined;
  queryClient.clear();
});

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useReconcilePersistedAgentSessions", () => {
  it("reconciles again when an authoritative list drops a remotely deleted session", async () => {
    const restoreLastSession = vi.fn();
    const reconcileSessions = vi.fn();
    restoreState = configureAgentSessionStatePort({
      restoreLastSession,
      reconcileSessions,
    } as unknown as AgentSessionStatePort);

    ({ unmount: unmountHook } = renderHook(() => useReconcilePersistedAgentSessions(), {
      wrapper,
    }));
    await waitFor(() => expect(reconcileSessions).toHaveBeenLastCalledWith(["ses_a", "ses_b"]));

    act(() => queryClient.setQueryData([AGENT_SESSIONS_KEY], [session("ses_b")]));

    await waitFor(() => expect(reconcileSessions).toHaveBeenLastCalledWith(["ses_b"]));
    expect(restoreLastSession).toHaveBeenCalledTimes(1);
    expect(reconcileSessions).toHaveBeenCalledTimes(2);
  });
});
