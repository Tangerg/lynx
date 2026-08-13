import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/public/queries";
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

async function waitForFileListRequest(index: number) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = transport.outbox().filter(({ method }) => method === "workspace.files.list")[
      index
    ];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for workspace.files.list request ${index + 1}`);
}

function file(path: string) {
  return {
    path,
    name: path,
    type: "file",
    modifiedAt: "2026-08-14T00:00:00Z",
  };
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
  registerDefaultDataProviders(createHost("file-list-provider-test", disposables));
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

describe("mounted Runtime Workspace-file-list generation", () => {
  it("aborts every old cwd page when the active Session retargets", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = renderHook(({ cwd }) => useWorkspaceListFiles({ cwd }), {
      wrapper,
      initialProps: { cwd: "/old" },
    });
    unmountHook = hook.unmount;

    const retiredFirst = await waitForFileListRequest(0);
    expect(retiredFirst.params).toEqual({ workspace: { path: "/old" } });
    respondSuccess(transport, retiredFirst.id, {
      data: [file("old-page-1.ts")],
      nextCursor: "old-page-2",
    });
    const retiredSecond = await waitForFileListRequest(1);
    expect(retiredSecond.params).toEqual({
      cursor: "old-page-2",
      workspace: { path: "/old" },
    });

    hook.rerender({ cwd: "/successor" });

    const successor = await waitForFileListRequest(2);
    expect(successor.params).toEqual({ workspace: { path: "/successor" } });
    respondSuccess(transport, successor.id, { data: [file("successor.ts")] });
    await waitFor(() => expect(hook.result.current.data?.[0]?.path).toBe("successor.ts"));

    const retiredSignal = send.mock.calls[0]?.[1];
    expect(retiredSignal).toBeInstanceOf(AbortSignal);
    expect(send.mock.calls[1]?.[1]).toBe(retiredSignal);
    expect(retiredSignal?.aborted).toBe(true);
    expect(send.mock.calls[2]?.[1]?.aborted).toBe(false);

    respondSuccess(transport, retiredSecond.id, { data: [file("old-page-2.ts")] });
    await act(async () => Promise.resolve());
    expect(hook.result.current.data?.[0]?.path).toBe("successor.ts");
    await expect(hook.result.current.promise).rejects.toThrow(
      "experimental_prefetchInRender feature flag is not enabled",
    );
  });
});
