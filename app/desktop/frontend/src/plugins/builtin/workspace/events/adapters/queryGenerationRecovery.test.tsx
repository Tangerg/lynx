import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { synchronizeMountedAgentSessions } = vi.hoisted(() => ({
  synchronizeMountedAgentSessions: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  AGENT_SESSIONS_KEY: "agent-sessions",
  AGENT_SESSION_USAGE_KEY: "agent-session-usage",
  synchronizeMountedAgentSessions,
}));

import { queryClient } from "@/lib/queryClient";
import { GOAL_KEY } from "@/plugins/builtin/chat/goal/public/queries";
import { createParameterizedDataQuery } from "@/plugins/sdk/dataQuery";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { definePlugin } from "@/plugins/sdk";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  retireWorkspaceReadModels,
} from "./queryInvalidation";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

interface GoalState {
  readonly available: boolean;
  readonly goal: { readonly objective: string } | null;
}

const useGoal = createParameterizedDataQuery<{ sessionId: string }, GoalState>(GOAL_KEY);

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function goal(objective: string): NonNullable<GoalState["goal"]> {
  return { objective };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

beforeEach(async () => {
  queryClient.clear();
  synchronizeMountedAgentSessions.mockClear();
});

afterEach(() => {
  queryClient.clear();
});

describe("workspace Runtime-generation query recovery", () => {
  it("revokes an old query before the successor event tail starts its replacement", async () => {
    const retired = deferred<GoalState>();
    let firstSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        firstSignal = signal;
        return retired.promise;
      }
      return Promise.resolve({ available: true, goal: goal("successor goal") });
    });
    await loadPluginsForTest(
      definePlugin({
        name: "test.goal-two-phase-generation-recovery",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: GOAL_KEY, fetcher });
        },
      }),
    );
    const { result } = renderHook(() => useGoal({ sessionId: "ses_goal_generation" }), {
      wrapper,
    });
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() => retireWorkspaceReadModels());

    expect(firstSignal?.aborted).toBe(true);
    expect(fetcher).toHaveBeenCalledOnce();
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "retire-live",
    });
    retired.resolve({ available: true, goal: goal("retired goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data).toBeUndefined();
    expect(fetcher).toHaveBeenCalledOnce();

    act(() => invalidateWorkspaceEverything());

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.data?.goal?.objective).toBe("successor goal"));
    await expect(result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });

  it("replaces an initial Goal read before the old Runtime settles", async () => {
    const retired = deferred<GoalState>();
    let firstSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        firstSignal = signal;
        return retired.promise;
      }
      return Promise.resolve({ available: true, goal: goal("successor goal") });
    });
    await loadPluginsForTest(
      definePlugin({
        name: "test.goal-generation-recovery",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: GOAL_KEY, fetcher });
        },
      }),
    );
    const { result } = renderHook(() => useGoal({ sessionId: "ses_goal_generation" }), {
      wrapper,
    });
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() => invalidateWorkspaceEverything());

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.data?.goal?.objective).toBe("successor goal"));
    expect(firstSignal?.aborted).toBe(true);
    expect(synchronizeMountedAgentSessions).toHaveBeenCalledWith({
      ownership: "replace-live",
    });

    retired.resolve({ available: true, goal: goal("retired goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data?.goal?.objective).toBe("successor goal");
    await expect(result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });

  it("replaces an initial Goal read when its committed change event arrives", async () => {
    const retired = deferred<GoalState>();
    let firstSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        firstSignal = signal;
        return retired.promise;
      }
      return Promise.resolve({ available: true, goal: goal("committed goal") });
    });
    await loadPluginsForTest(
      definePlugin({
        name: "test.goal-event-generation-recovery",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: GOAL_KEY, fetcher });
        },
      }),
    );
    const { result } = renderHook(() => useGoal({ sessionId: "ses_goal_generation" }), {
      wrapper,
    });
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() =>
      invalidateWorkspaceEvent({
        type: "goals.changed",
        sequence: 1,
        sessionIds: ["ses_goal_generation"],
      }),
    );

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.data?.goal?.objective).toBe("committed goal"));
    expect(firstSignal?.aborted).toBe(true);

    retired.resolve({ available: true, goal: goal("pre-event goal") });
    await act(async () => Promise.resolve());
    expect(result.current.data?.goal?.objective).toBe("committed goal");
    await expect(result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });
});
