import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { updateProvider } from "./application/providerConfig";
import { PROVIDERS_KEY } from "./application/providerQueries";
import type { ProviderConfiguration } from "./application/providerModels";
import providersPlugin from "./index";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
});

describe("providers plugin Runtime generation wiring", () => {
  it("retires an admitted command when the Runtime process generation changes", async () => {
    const retired = deferred<ProviderConfiguration>();
    const update = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ providers: { update } }) as unknown as LyraClient,
    });
    let generation = "runtime_1";
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM_PORTS },
      setup() {
        return {
          stream: {
            runtimeGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            verifyServiceConnection: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, providersPlugin);
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const command = rejected(
      updateProvider({
        provider: "openai-compatible",
        baseUrl: "https://retired.example.test/v1",
      }),
    );
    await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());

    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });

    retired.resolve(provider({ baseUrl: "https://retired.example.test/v1" }));
    await Promise.resolve();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([provider()]);
  });
});

function provider(overrides: Partial<ProviderConfiguration> = {}): ProviderConfiguration {
  return {
    id: "openai-compatible",
    baseUrl: "",
    apiKeyMasked: "",
    requiresBaseUrl: true,
    embeddingCapable: true,
    defaultEmbeddingModel: "embed-1",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
