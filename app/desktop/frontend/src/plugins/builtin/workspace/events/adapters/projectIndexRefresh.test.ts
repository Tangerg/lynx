import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import {
  WORKSPACE_PROJECTS_KEY,
  useWorkspaceProjects,
} from "@/plugins/builtin/workspace/public/queries";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import type { Disposable } from "@/plugins/sdk";
import { installProjectIndexRefresh, workspaceProjectRevision } from "./projectIndexRefresh";
import { contributeForTest } from "@/plugins/sdk/testKernel";

let disposables: Disposable[] = [];
let disposeRefresh: (() => void) | undefined;
let unmountHook: (() => void) | undefined;

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function session(
  patch: Omit<Partial<AgentSessionSummary>, "workspace"> & { cwd?: string } = {},
): AgentSessionSummary {
  const { cwd = "/repo", ...summary } = patch;
  return {
    id: "ses_1",
    revision: 1,
    title: "Session",
    status: "idle",
    provider: "provider",
    model: "model",
    workspace: { path: cwd, availability: "available" },
    time: "2026-08-11T00:00:00Z",
    ...summary,
  };
}

beforeEach(async () => {
  queryClient.clear();
  disposables = [];
});

afterEach(() => {
  unmountHook?.();
  unmountHook = undefined;
  disposeRefresh?.();
  disposeRefresh = undefined;
  queryClient.clear();
  for (const disposable of disposables.reverse()) disposable.dispose();
  vi.restoreAllMocks();
});

describe("workspaceProjectRevision", () => {
  it("tracks only the Session facts that determine workspaces.list", () => {
    const baseline = workspaceProjectRevision([session()]);

    expect(
      workspaceProjectRevision([
        session({ status: "running", revision: 2, title: "Renamed", model: "other" }),
      ]),
    ).toBe(baseline);
    expect(workspaceProjectRevision([session({ cwd: "/elsewhere" })])).not.toBe(baseline);
    expect(workspaceProjectRevision([session({ time: "2026-08-11T00:01:00Z" })])).not.toBe(
      baseline,
    );
    expect(workspaceProjectRevision([session({ id: "ses_2" })])).not.toBe(baseline);
  });

  it("replaces an initial project read after its Session projection commits", async () => {
    const retired = deferred<Array<{ id: string; name: string; sessionCount: number }>>();
    let retiredSignal: AbortSignal | undefined;
    let calls = 0;
    const fetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      calls += 1;
      if (calls === 1) {
        retiredSignal = signal;
        return retired.promise;
      }
      return Promise.resolve([{ id: "/successor", name: "successor", sessionCount: 1 }]);
    });
    await contributeForTest((ctx) => {
      ctx.contribute(DATA_PROVIDER, { key: WORKSPACE_PROJECTS_KEY, fetcher });
    });
    disposeRefresh = installProjectIndexRefresh();
    const hook = renderHook(() => useWorkspaceProjects(), { wrapper });
    unmountHook = hook.unmount;
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());

    act(() => {
      queryClient.setQueryData([AGENT_SESSIONS_KEY], [session({ id: "ses_committed" })]);
    });

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(hook.result.current.data?.[0]?.id).toBe("/successor"));
    expect(retiredSignal?.aborted).toBe(true);

    retired.resolve([{ id: "/retired", name: "retired", sessionCount: 0 }]);
    await act(async () => Promise.resolve());
    expect(hook.result.current.data?.[0]?.id).toBe("/successor");
  });
});
