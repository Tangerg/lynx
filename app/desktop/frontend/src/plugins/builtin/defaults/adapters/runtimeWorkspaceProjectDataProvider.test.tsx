import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import {
  WORKSPACE_PROJECTS_KEY,
  useWorkspaceProjects,
} from "@/plugins/builtin/workspace/public/queries";
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

async function waitForProjectRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "workspaces.list")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for workspaces.list request ${index + 1}`);
}

function workspace(path: string) {
  return {
    workspace: {
      ref: { path },
      projectRoot: path,
      availability: "available",
    },
    name: path.slice(1),
    sessionCount: 1,
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

describe("mounted Runtime Workspace-project generation", () => {
  it("aborts the retired project request before its replacement commits", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(() => useWorkspaceProjects(), { wrapper });
    unmountHook = hook.unmount;
    const retired = await waitForProjectRequest(0);

    act(() => {
      void queryClient.cancelQueries({ queryKey: [WORKSPACE_PROJECTS_KEY] });
      void queryClient.invalidateQueries({ queryKey: [WORKSPACE_PROJECTS_KEY] });
    });

    const successor = await waitForProjectRequest(1);
    respondSuccess(transport, successor.id, { data: [workspace("/successor")] });
    await waitFor(() => expect(hook.result.current.data?.[0]?.id).toBe("/successor"));

    const retiredSignal = send.mock.calls[0]?.[1];
    expect(retiredSignal).toBeInstanceOf(AbortSignal);
    expect(retiredSignal?.aborted).toBe(true);
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(false);

    respondSuccess(transport, retired.id, { data: [workspace("/retired")] });
    await act(async () => Promise.resolve());
    expect(hook.result.current.data?.[0]?.id).toBe("/successor");
    await expect(hook.result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });
});
