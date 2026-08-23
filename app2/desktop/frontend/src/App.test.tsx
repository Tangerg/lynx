import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  DiscoverResponse,
  RuntimeConnection,
} from "@lyra/runtime-contract";

import { App } from "./App";

const runtimeMocks = vi.hoisted(() => ({
  bootstrap: vi.fn(),
  discover: vi.fn(),
}));

vi.mock("./runtime/desktopBridge", () => ({
  loadDesktopBootstrap: runtimeMocks.bootstrap,
  useLocalRuntime: vi.fn(),
}));

vi.mock("./runtime/runtimeQueries", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("./runtime/runtimeQueries")>();
  return { ...actual, discoverRuntime: runtimeMocks.discover };
});

vi.mock("./features/workspace/WorkspaceShell", () => ({
  WorkspaceShell: ({ connection }: { connection: RuntimeConnection }) => (
    <dl>
      <dt>Generation</dt>
      <dd>{connection.generation}</dd>
    </dl>
  ),
}));

function connection(generation: number): RuntimeConnection {
  return {
    endpoint: `http://127.0.0.1:${32_000 + generation}`,
    bearerToken: `token-${generation}`,
    instanceId: `ins_${generation}`,
    protocolVersion: "2026-08-23",
    idempotencyNamespace: `idp_${generation}`,
    generation,
  };
}

function discovery(generation: number): DiscoverResponse {
  return {
    protocolVersion: "2026-08-23",
    serverInfo: {
      instanceId: `ins_${generation}`,
      name: "lyra-runtime",
      version: "dev",
      defaultWorkspace: { path: "/workspace" },
      home: "/home/test",
    },
    capabilities: {
      runEvents: [],
      runtimeTopics: [],
      streamingMethods: [],
      features: {},
      limits: {
        idempotency: {
          retentionSeconds: 86_400,
          namespace: `idp_${generation}`,
        },
        runReplay: {
          scope: "runtimeInstanceRootSegment",
          maxEvents: 10_000,
          maxBytes: 67_108_864,
        },
        mcpAuthorizationAttempts: { retentionSeconds: 600 },
        runtimeSubscription: { maxTopics: 64, maxWatches: 256 },
      },
    },
  };
}

afterEach(() => {
  runtimeMocks.bootstrap.mockReset();
  runtimeMocks.discover.mockReset();
});

describe("App Runtime generations", () => {
  it("does not let a predecessor discovery overwrite its successor", async () => {
    let resolveFirst: ((value: DiscoverResponse) => void) | undefined;
    const firstDiscovery = new Promise<DiscoverResponse>((resolve) => {
      resolveFirst = resolve;
    });
    runtimeMocks.bootstrap
      .mockResolvedValueOnce({ runtime: connection(1) })
      .mockResolvedValue({ runtime: connection(2) });
    runtimeMocks.discover.mockImplementation((runtime: RuntimeConnection) =>
      runtime.generation === 1 ? firstDiscovery : Promise.resolve(discovery(2)),
    );
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, refetchInterval: false } },
    });
    const view = render(
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>,
    );

    await waitFor(() =>
      expect(runtimeMocks.discover).toHaveBeenCalledWith(
        connection(1),
        expect.any(AbortSignal),
      ),
    );
    await queryClient.invalidateQueries({ queryKey: ["desktop", "bootstrap"] });
    await waitFor(() =>
      expect(runtimeMocks.discover).toHaveBeenCalledWith(
        connection(2),
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByText("Generation").nextElementSibling?.textContent).toBe(
      "2",
    );

    resolveFirst?.(discovery(1));
    await waitFor(() =>
      expect(
        screen.getByText("Generation").nextElementSibling?.textContent,
      ).toBe("2"),
    );

    view.unmount();
    queryClient.clear();
  });
});
