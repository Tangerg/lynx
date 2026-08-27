import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { createScopeAppClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess } from "@/rpc/transports/memory.testkit";
import { USAGE_SUMMARY_KEY, useUsageReport } from "../application/usageConfig";
import { installUsageGateway } from "./runtimeUsageGateway";

let queryClient: QueryClient;
let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createScopeAppClient>;
let restoreGateway: (() => void) | undefined;
let unmountHook: (() => void) | undefined;

async function waitForUsageRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "usage.summary")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for usage.summary request ${index + 1}`);
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  transport = createMemoryTransport();
  client = createScopeAppClient(transport);
  setContainer({ client: () => client });
  restoreGateway = installUsageGateway();
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  restoreGateway?.();
  restoreGateway = undefined;
  queryClient.clear();
  await client.close();
  await resetContainer();
  vi.restoreAllMocks();
});

describe("mounted Usage summary generation", () => {
  it("aborts the pre-event transport before committing the successor read", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(() => useUsageReport(30), { wrapper });
    unmountHook = hook.unmount;
    const first = await waitForUsageRequest(0);
    expect(first.params).toEqual({ sinceDays: 30 });
    const firstSignal = send.mock.calls[0]?.[1];

    act(() => {
      void queryClient.cancelQueries({ queryKey: [USAGE_SUMMARY_KEY, 30], exact: true });
      void queryClient.invalidateQueries({ queryKey: [USAGE_SUMMARY_KEY, 30], exact: true });
    });

    const second = await waitForUsageRequest(1);
    respondSuccess(transport, second.id, {
      total: { inputTokens: 34, outputTokens: 13 },
      sessions: 2,
      runs: 3,
    });
    await waitFor(() => expect(hook.result.current.data?.total.inputTokens).toBe(34));
    respondSuccess(transport, first.id, {
      total: { inputTokens: 1, outputTokens: 1 },
      sessions: 1,
      runs: 1,
    });
    await act(async () => Promise.resolve());

    expect(firstSignal).toBeInstanceOf(AbortSignal);
    expect(firstSignal?.aborted).toBe(true);
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(false);
    expect(hook.result.current.data?.total.inputTokens).toBe(34);
  });
});
