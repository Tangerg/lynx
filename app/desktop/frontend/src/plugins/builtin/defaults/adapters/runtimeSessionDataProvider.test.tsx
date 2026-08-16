import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import { AGENT_SESSIONS_KEY, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import type { Disposable } from "@/plugins/sdk";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess } from "@/rpc/transports/memory.testkit";
import { registerDefaultDataProviders } from "./runtimeDataProviders";
import { contributeForTest } from "@/plugins/sdk/testKernel";

let disposables: Disposable[] = [];
let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createLyraClient>;
let unmountHook: (() => void) | undefined;

async function waitForSessionRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "sessions.list")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for sessions.list request ${index + 1}`);
}

function session(id: string) {
  return {
    id,
    title: id,
    status: "idle",
    model: "test-model",
    workspace: {
      ref: { path: "/repo" },
      projectRoot: "/repo",
      availability: "available",
    },
    createdAt: "2026-08-14T00:00:00Z",
    updatedAt: "2026-08-14T00:00:00Z",
    revision: 1,
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(async () => {
  queryClient.clear();
  disposables = [];
  transport = createMemoryTransport();
  client = createLyraClient(transport);
  setContainer({ client: () => client });
  await contributeForTest(registerDefaultDataProviders);
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  queryClient.clear();
  for (const disposable of disposables.reverse()) disposable.dispose();
  await client.close();
  await resetContainer();
  vi.restoreAllMocks();
});

describe("mounted Runtime Session-list generation", () => {
  it("aborts every retired page before the replacement read commits", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(() => useAgentSessions(), { wrapper });
    unmountHook = hook.unmount;

    const retiredFirst = await waitForSessionRequest(0);
    respondSuccess(transport, retiredFirst.id, {
      data: [session("retired-page-1")],
      nextCursor: "retired-page-2",
    });
    const retiredSecond = await waitForSessionRequest(1);
    expect(retiredSecond.params).toEqual({ cursor: "retired-page-2" });

    act(() => {
      void queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] });
      void queryClient.invalidateQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    });

    const successor = await waitForSessionRequest(2);
    respondSuccess(transport, successor.id, { data: [session("successor")] });
    await waitFor(() =>
      expect(hook.result.current.data?.map(({ id }) => id)).toEqual(["successor"]),
    );

    const retiredSignal = send.mock.calls[0]?.[1];
    expect(retiredSignal).toBeInstanceOf(AbortSignal);
    expect(send.mock.calls[1]?.[1]).toBe(retiredSignal);
    expect(retiredSignal?.aborted).toBe(true);
    expect(send.mock.calls[2]?.[1]?.aborted).toBe(false);

    respondSuccess(transport, retiredSecond.id, { data: [session("retired-page-2")] });
    await act(async () => Promise.resolve());
    expect(hook.result.current.data?.map(({ id }) => id)).toEqual(["successor"]);
    await expect(hook.result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });
});
