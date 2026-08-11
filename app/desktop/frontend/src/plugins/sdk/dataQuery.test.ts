import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "./kernelPoints";
import { definePlugin, loadPlugin } from "./definePlugin";
import { createParameterizedDataQuery } from "./dataQuery";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

async function registerProvider(fetcher: (params: unknown) => Promise<string>): Promise<void> {
  await loadPlugin(
    definePlugin({
      name: "test.parameterized-data-query",
      version: "1.0.0",
      setup({ host }) {
        host.extensions.contribute(DATA_PROVIDER, { key: "resource", fetcher });
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
      wrapper,
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

    const { result } = renderHook(() => useResource(undefined), { wrapper });

    expect(result.current.fetchStatus).toBe("idle");
    expect(fetcher).not.toHaveBeenCalled();
  });
});
