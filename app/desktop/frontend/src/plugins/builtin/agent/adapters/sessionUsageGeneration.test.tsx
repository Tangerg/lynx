import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess } from "@/rpc/transports/memory.testkit";
import { AGENT_SESSION_USAGE_KEY, useAgentSessionUsage } from "../application/session/sessionUsage";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";
import { queryClient } from "@/lib/queryClient";

let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createLyraClient>;
let restoreGateway: ReturnType<typeof installAgentRuntimeGateway> | undefined;
let unmountHook: (() => void) | undefined;
let restoreQueryDefaults: (() => void) | undefined;

async function waitForUsageRequest(
  index: number,
  source: ReturnType<typeof createMemoryTransport> = transport,
) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = source.outbox().filter(({ method }) => method === "usage.session")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for usage.session request ${index + 1}`);
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  const defaults = queryClient.getDefaultOptions();
  queryClient.setDefaultOptions({
    ...defaults,
    queries: {
      ...defaults.queries,
      experimental_prefetchInRender: true,
      retry: false,
    },
  });
  restoreQueryDefaults = () => queryClient.setDefaultOptions(defaults);
  queryClient.clear();
  transport = createMemoryTransport();
  client = createLyraClient(transport);
  setContainer({ client: () => client });
  restoreGateway = installAgentRuntimeGateway();
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  restoreGateway?.dispose();
  restoreGateway = undefined;
  queryClient.clear();
  restoreQueryDefaults?.();
  restoreQueryDefaults = undefined;
  await client.close();
  await resetContainer();
  vi.restoreAllMocks();
});

describe("mounted Session usage generation", () => {
  it("does not let an empty handoff settlement invalidate a later query", async () => {
    restoreGateway?.dispose();
    restoreGateway = undefined;
    await act(async () => Promise.resolve());
    queryClient.clear();

    let releaseHandoff!: () => void;
    const handoffSettlement = new Promise<void>((resolve) => {
      releaseHandoff = resolve;
    });
    const cancelQueries = vi
      .spyOn(queryClient, "cancelQueries")
      .mockReturnValueOnce(handoffSettlement);
    restoreGateway = installAgentRuntimeGateway();

    const hook = renderHook(() => useAgentSessionUsage("ses_future"), { wrapper });
    unmountHook = hook.unmount;
    const request = await waitForUsageRequest(0);
    respondSuccess(transport, request.id, { inputTokens: 13, outputTokens: 5 });
    await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(13));

    await act(async () => {
      releaseHandoff();
      await handoffSettlement;
      await Promise.resolve();
    });

    expect(transport.outbox().filter(({ method }) => method === "usage.session")).toHaveLength(1);
    expect(cancelQueries).not.toHaveBeenCalled();
  });

  it("aborts the pre-event transport before committing the successor read", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(() => useAgentSessionUsage("ses_usage"), { wrapper });
    unmountHook = hook.unmount;
    const first = await waitForUsageRequest(0);
    const firstSignal = send.mock.calls[0]?.[1];

    act(() => {
      void queryClient.cancelQueries({
        queryKey: [AGENT_SESSION_USAGE_KEY, "ses_usage"],
        exact: true,
      });
      void queryClient.invalidateQueries({
        queryKey: [AGENT_SESSION_USAGE_KEY, "ses_usage"],
        exact: true,
      });
    });

    const second = await waitForUsageRequest(1);
    respondSuccess(transport, second.id, { inputTokens: 21, outputTokens: 8 });
    await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(21));
    respondSuccess(transport, first.id, { inputTokens: 1, outputTokens: 1 });
    await act(async () => Promise.resolve());

    expect(firstSignal).toBeInstanceOf(AbortSignal);
    expect(firstSignal?.aborted).toBe(true);
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(false);
    expect(hook.result.current.data?.inputTokens).toBe(21);
  });

  it("hands the cache writer to the successor Runtime gateway", async () => {
    const retiredSend = vi.spyOn(transport, "send");
    const hook = renderHook(() => useAgentSessionUsage("ses_usage"), { wrapper });
    unmountHook = hook.unmount;
    const retiredRequest = await waitForUsageRequest(0);
    const retiredSignal = retiredSend.mock.calls[0]?.[1];

    const successorTransport = createMemoryTransport();
    const successorClient = createLyraClient(successorTransport);
    const successorSend = vi.spyOn(successorTransport, "send");
    setContainer({ client: () => successorClient });
    let disposeSuccessor!: ReturnType<typeof installAgentRuntimeGateway>;
    await act(async () => {
      disposeSuccessor = installAgentRuntimeGateway();
      await Promise.resolve();
    });
    restoreGateway?.dispose();
    restoreGateway = undefined;
    try {
      const successorRequest = await waitForUsageRequest(0, successorTransport);
      respondSuccess(successorTransport, successorRequest.id, {
        inputTokens: 34,
        outputTokens: 13,
      });
      await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(34));

      expect(retiredSignal?.aborted).toBe(true);
      expect(successorSend.mock.calls[0]?.[1]?.aborted).toBe(false);
      respondSuccess(transport, retiredRequest.id, { inputTokens: 1, outputTokens: 1 });
      await act(async () => Promise.resolve());
      expect(hook.result.current.data?.inputTokens).toBe(34);
    } finally {
      disposeSuccessor.dispose();
      await successorClient.close();
    }
  });
});
