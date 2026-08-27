import {
  defaultScheduler,
  notifyManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Host } from "dougong";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "./kernelPoints";
import { definePlugin } from "./definePlugin";
import { createParameterizedDataQuery } from "./dataQuery";
import { addPluginsForTest, loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { startKernel, stopKernel } from "./bootstrap";

const ownedHosts: Host[] = [];
let restoreProductQueryDefaults: (() => void) | undefined;

beforeEach(() => {
  const defaults = queryClient.getDefaultOptions();
  queryClient.setDefaultOptions({
    ...defaults,
    queries: {
      ...defaults.queries,
      retry: false,
    },
  });
  restoreProductQueryDefaults = () => queryClient.setDefaultOptions(defaults);
});

afterEach(async () => {
  queryClient.clear();
  await Promise.allSettled(ownedHosts.splice(0).reverse().map(stopKernel));
  restoreProductQueryDefaults?.();
  restoreProductQueryDefaults = undefined;
});

function createWrapper() {
  return function Wrapper({ children }: { children: ReactNode }) {
    const [client] = useState(
      () =>
        new QueryClient({
          defaultOptions: {
            queries: {
              retry: false,
              gcTime: Infinity,
            },
          },
        }),
    );
    return createElement(QueryClientProvider, { client }, children);
  };
}

function productQueryWrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

async function registerProvider(fetcher: (params: unknown) => Promise<string>): Promise<void> {
  await loadPluginsForTest(
    definePlugin({
      name: "test.parameterized-data-query",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher });
      },
    }),
  );
}

describe("createParameterizedDataQuery", () => {
  it("does not carry data across parameter identities", async () => {
    let resolveNew: ((value: string) => void) | undefined;
    await registerProvider(
      vi.fn((params: unknown) => {
        const workspace = (params as { workspace: string }).workspace;
        if (workspace === "/work/old") return Promise.resolve("old value");
        return new Promise<string>((resolve) => {
          resolveNew = resolve;
        });
      }),
    );
    const useResource = createParameterizedDataQuery<{ workspace: string }, string>("resource");

    const { result, rerender } = renderHook(({ workspace }) => useResource({ workspace }), {
      initialProps: { workspace: "/work/old" },
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.data).toBe("old value"));

    rerender({ workspace: "/work/new" });

    expect(result.current.data).toBeUndefined();
    resolveNew?.("new value");
    await waitFor(() => expect(result.current.data).toBe("new value"));
  });

  it("keeps an absent parameter disabled", async () => {
    const fetcher = vi.fn().mockResolvedValue("unexpected");
    await registerProvider(fetcher);
    const useResource = createParameterizedDataQuery<{ id: string }, string>("resource");

    const { result } = renderHook(() => useResource(undefined), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("hands a mounted cache writer to the successor Plugin Host", async () => {
    const retired = deferred<string>();
    let retiredSignal: AbortSignal | undefined;
    const retiredFetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      retiredSignal = signal;
      return retired.promise;
    });
    const successorFetcher = vi.fn().mockResolvedValue("successor value");
    ownedHosts.push(
      await startKernel([
        definePlugin({
          name: "test.retired-data-provider",
          setup(ctx) {
            ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: retiredFetcher });
          },
        }),
      ]),
    );
    const useResource = createParameterizedDataQuery<{ id: string }, string>("resource");
    const hook = renderHook(() => useResource({ id: "same" }), {
      wrapper: productQueryWrapper,
    });
    await waitFor(() => expect(retiredFetcher).toHaveBeenCalledOnce());

    ownedHosts.push(
      await startKernel([
        definePlugin({
          name: "test.successor-data-provider",
          setup(ctx) {
            ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: successorFetcher });
          },
        }),
      ]),
    );
    await stopKernel(ownedHosts.shift()!);

    await waitFor(() => expect(successorFetcher).toHaveBeenCalledOnce());
    await waitFor(() => expect(hook.result.current.data).toBe("successor value"));
    expect(retiredSignal?.aborted).toBe(true);

    retired.resolve("retired value");
    await act(async () => Promise.resolve());
    expect(hook.result.current.data).toBe("successor value");
  });

  it("clears cached provider data before the successor settles", async () => {
    ownedHosts.push(
      await startKernel([
        definePlugin({
          name: "test.cached-retired-data-provider",
          setup(ctx) {
            ctx.contribute(DATA_PROVIDER, {
              key: "resource",
              fetcher: vi.fn().mockResolvedValue("cached retired value"),
            });
          },
        }),
      ]),
    );
    const successor = deferred<string>();
    const successorFetcher = vi.fn(() => successor.promise);
    const useResource = createParameterizedDataQuery<{ id: string }, string>("resource");
    const hook = renderHook(() => useResource({ id: "same" }), {
      wrapper: productQueryWrapper,
    });
    await waitFor(() => expect(hook.result.current.data).toBe("cached retired value"));

    ownedHosts.push(
      await startKernel([
        definePlugin({
          name: "test.cached-successor-data-provider",
          setup(ctx) {
            ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: successorFetcher });
          },
        }),
      ]),
    );

    await waitFor(() => expect(successorFetcher).toHaveBeenCalledOnce());
    expect(hook.result.current.data).toBeUndefined();
    successor.resolve("fresh successor value");
    await waitFor(() => expect(hook.result.current.data).toBe("fresh successor value"));
  });

  it("removes the retired writer after the renderer and current Host finally close", async () => {
    const retired = deferred<string>();
    let retiredSignal: AbortSignal | undefined;
    const retiredFetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      retiredSignal = signal;
      return retired.promise;
    });
    const host = await startKernel([
      definePlugin({
        name: "test.final-data-provider",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: retiredFetcher });
        },
      }),
    ]);
    ownedHosts.push(host);
    const specDefaults = queryClient.getDefaultOptions();
    const params = { id: "same" };
    const useResource = createParameterizedDataQuery<typeof params, string>("resource");
    const hook = renderHook(() => useResource(params), {
      wrapper: productQueryWrapper,
    });
    await waitFor(() => expect(retiredFetcher).toHaveBeenCalledOnce());
    hook.unmount();
    expect(
      queryClient.getQueryCache().find({ queryKey: ["resource", params], exact: true }),
    ).toBeDefined();

    // Vitest's async-resource collector cannot join TanStack's ref'ed
    // setTimeout(0) notifier. The documented scheduler seam keeps this spec's
    // observer settlement owned and is restored before leaving the test.
    notifyManager.setScheduler(queueMicrotask);
    try {
      await act(async () => stopKernel(ownedHosts.pop()!));

      expect(retiredSignal?.aborted).toBe(true);
      await waitFor(() =>
        expect(
          queryClient.getQueryCache().find({ queryKey: ["resource", params], exact: true }),
        ).toBeUndefined(),
      );
      retired.resolve("retired value");
      await act(async () => Promise.resolve());
      expect(
        queryClient.getQueryCache().find({ queryKey: ["resource", params], exact: true }),
      ).toBeUndefined();
    } finally {
      notifyManager.setScheduler(defaultScheduler);
      queryClient.setDefaultOptions(specDefaults);
    }
  });

  it("replaces one provider overridden inside the active Host", async () => {
    const retired = deferred<string>();
    let retiredSignal: AbortSignal | undefined;
    const retiredFetcher = vi.fn((_params?: unknown, signal?: AbortSignal) => {
      retiredSignal = signal;
      return retired.promise;
    });
    const host = await startKernel([
      definePlugin({
        name: "test.base-data-provider",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: retiredFetcher });
        },
      }),
    ]);
    ownedHosts.push(host);
    const successorFetcher = vi.fn().mockResolvedValue("override value");
    const useResource = createParameterizedDataQuery<{ id: string }, string>("resource");
    const hook = renderHook(() => useResource({ id: "same" }), {
      wrapper: productQueryWrapper,
    });
    await waitFor(() => expect(retiredFetcher).toHaveBeenCalledOnce());

    await addPluginsForTest(host, [
      definePlugin({
        name: "test.override-data-provider",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: successorFetcher });
        },
      }),
    ]);

    await waitFor(() => expect(successorFetcher).toHaveBeenCalledOnce());
    await waitFor(() => expect(hook.result.current.data).toBe("override value"));
    expect(retiredSignal?.aborted).toBe(true);
    retired.resolve("retired value");
    await act(async () => Promise.resolve());
    expect(hook.result.current.data).toBe("override value");
  });

  it("keeps an unchanged provider cache when another key is contributed", async () => {
    const resourceFetcher = vi.fn().mockResolvedValue("stable value");
    const host = await startKernel([
      definePlugin({
        name: "test.stable-data-provider",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: "resource", fetcher: resourceFetcher });
        },
      }),
    ]);
    ownedHosts.push(host);
    const useResource = createParameterizedDataQuery<{ id: string }, string>("resource");
    const hook = renderHook(() => useResource({ id: "same" }), {
      wrapper: productQueryWrapper,
    });
    await waitFor(() => expect(hook.result.current.data).toBe("stable value"));
    await act(async () => Promise.resolve());
    expect(resourceFetcher).toHaveBeenCalledOnce();

    await addPluginsForTest(host, [
      definePlugin({
        name: "test.unrelated-data-provider",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, {
            key: "unrelated",
            fetcher: vi.fn().mockResolvedValue("unrelated value"),
          });
        },
      }),
    ]);
    await act(async () => Promise.resolve());

    expect(resourceFetcher).toHaveBeenCalledOnce();
    expect(hook.result.current.data).toBe("stable value");
  });
});
