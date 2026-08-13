import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess } from "@/rpc/transports/memory.testkit";
import { AGENT_SESSION_USAGE_KEY, useAgentSessionUsage } from "../application/session/sessionUsage";
import { installAgentRuntimeGateway } from "./agentRuntimeGateway";

let queryClient: QueryClient;
let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createLyraClient>;
let restoreGateway: (() => void) | undefined;
let unmountHook: (() => void) | undefined;

async function waitForUsageRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "usage.session")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for usage.session request ${index + 1}`);
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { experimental_prefetchInRender: true, retry: false } },
  });
  transport = createMemoryTransport();
  client = createLyraClient(transport);
  setContainer({ client: () => client });
  restoreGateway = installAgentRuntimeGateway();
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

describe("mounted Session usage generation", () => {
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
});
