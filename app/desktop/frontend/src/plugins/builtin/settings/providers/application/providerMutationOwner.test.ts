import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { ProviderMutationOwner } from "./providerMutationOwner";
import type { ProviderGateway } from "./ports/providerGateway";
import { MODELS_KEY, PROVIDERS_KEY } from "./providerQueries";
import type { ProviderConfiguration } from "./providerModels";

let owner: ProviderMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
  queryClient.removeQueries({ queryKey: [MODELS_KEY] });
  vi.restoreAllMocks();
});

describe("ProviderMutationOwner", () => {
  it("publishes one material generation for install, Runtime replacement, and final disposal", () => {
    const start = ProviderMutationOwner.materialGeneration();
    owner = ProviderMutationOwner.install({} as ProviderGateway);
    expect(ProviderMutationOwner.materialGeneration()).toBe(start + 1);

    owner.replaceRuntimeGeneration();
    expect(ProviderMutationOwner.materialGeneration()).toBe(start + 2);

    owner.dispose();
    expect(ProviderMutationOwner.materialGeneration()).toBe(start + 3);
    owner = undefined;
  });

  it("retires one Runtime generation without settling its commands into the successor", async () => {
    const retired = deferred<ProviderConfiguration>();
    const updateProvider = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce(provider({ baseUrl: "https://successor.example.test/v1" }));
    owner = ProviderMutationOwner.install({ updateProvider } as unknown as ProviderGateway);
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const inFlight = owner.updateProvider({
      provider: "openai-compatible",
      baseUrl: "https://retired.example.test/v1",
    });
    const queued = owner.updateProvider({
      provider: "openai-compatible",
      baseUrl: "https://queued.example.test/v1",
    });
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateProvider).toHaveBeenCalledOnce());

    owner.replaceRuntimeGeneration();
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    expect(updateProvider).toHaveBeenCalledOnce();

    retired.resolve(provider({ baseUrl: "https://retired.example.test/v1" }));
    await Promise.resolve();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([provider()]);

    await expect(
      owner.updateProvider({
        provider: "openai-compatible",
        baseUrl: "https://successor.example.test/v1",
      }),
    ).resolves.toEqual(provider({ baseUrl: "https://successor.example.test/v1" }));
    expect(updateProvider).toHaveBeenCalledTimes(2);
  });

  it("does not globally serialize unrelated provider resources", async () => {
    const first = deferred<ProviderConfiguration>();
    const updateProvider = vi.fn((input: { provider: string }) =>
      input.provider === "openai"
        ? first.promise
        : Promise.resolve(provider({ id: input.provider, baseUrl: "https://independent.test" })),
    );
    owner = ProviderMutationOwner.install({ updateProvider } as unknown as ProviderGateway);

    const blocked = owner.updateProvider({ provider: "openai", baseUrl: "https://blocked.test" });
    const independent = owner.updateProvider({
      provider: "deepseek",
      baseUrl: "https://independent.test",
    });

    await vi.waitFor(() => expect(updateProvider).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toMatchObject({ id: "deepseek" });
    first.resolve(provider({ id: "openai", baseUrl: "https://blocked.test" }));
    await expect(blocked).resolves.toMatchObject({ id: "openai" });
  });

  it("does not turn failed cache repair into an accepted provider command failure", async () => {
    const saved = provider({ baseUrl: "https://saved.example.test/v1" });
    owner = ProviderMutationOwner.install({
      updateProvider: vi.fn().mockResolvedValue(saved),
    } as unknown as ProviderGateway);
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(
      owner.updateProvider({ provider: saved.id, baseUrl: saved.baseUrl }),
    ).resolves.toEqual(saved);
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([saved]);
  });

  it("retires a live provider probe without publishing its late result", async () => {
    const probe = deferred<{ ok: boolean }>();
    owner = ProviderMutationOwner.install({
      testProvider: vi.fn(() => probe.promise),
    } as unknown as ProviderGateway);

    const result = rejected(owner.testProvider("openai"));
    owner.replaceRuntimeGeneration();
    await expect(result).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    probe.resolve({ ok: true });
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
