import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import { useWorkspaceFileChanges } from "@/plugins/builtin/workspace/public/queries";
import { createHost } from "@/plugins/sdk/host";
import { usePluginStore } from "@/plugins/sdk/registry";
import type { Disposable } from "@/plugins/sdk";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess } from "@/rpc/transports/memory.testkit";
import { registerDefaultDataProviders } from "./runtimeDataProviders";

let disposables: Disposable[] = [];
let transport: ReturnType<typeof createMemoryTransport>;
let client: ReturnType<typeof createLyraClient>;
let unmountHook: (() => void) | undefined;

async function waitForChangesRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "workspace.changes.list")[
      index
    ];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for workspace.changes.list request ${index + 1}`);
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  queryClient.clear();
  usePluginStore.getState().resetForTest();
  disposables = [];
  transport = createMemoryTransport();
  client = createLyraClient(transport);
  setContainer({ client: () => client });
  registerDefaultDataProviders(createHost("file-changes-provider-test", disposables));
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  queryClient.clear();
  for (const disposable of disposables.reverse()) disposable.dispose();
  await client.close();
  await resetContainer();
  usePluginStore.getState().resetForTest();
  vi.restoreAllMocks();
});

describe("mounted Runtime Workspace-file-changes generation", () => {
  it("aborts the old cwd request when the active Session retargets", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(({ cwd }) => useWorkspaceFileChanges({ cwd }), {
      wrapper,
      initialProps: { cwd: "/old" },
    });
    unmountHook = hook.unmount;
    const retired = await waitForChangesRequest(0);
    expect(retired.params).toEqual({ workspace: { path: "/old" } });

    hook.rerender({ cwd: "/successor" });

    const successor = await waitForChangesRequest(1);
    expect(successor.params).toEqual({ workspace: { path: "/successor" } });
    respondSuccess(transport, successor.id, {
      data: [{ path: "successor.ts", status: "modified", added: 7, removed: 2 }],
    });
    await waitFor(() => expect(hook.result.current.data?.[0]?.path).toBe("successor.ts"));

    const retiredSignal = send.mock.calls[0]?.[1];
    expect(retiredSignal).toBeInstanceOf(AbortSignal);
    expect(retiredSignal?.aborted).toBe(true);
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(false);

    respondSuccess(transport, retired.id, {
      data: [{ path: "retired.ts", status: "modified", added: 1, removed: 1 }],
    });
    await act(async () => Promise.resolve());
    expect(hook.result.current.data?.[0]?.path).toBe("successor.ts");
    await expect(hook.result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });
});
