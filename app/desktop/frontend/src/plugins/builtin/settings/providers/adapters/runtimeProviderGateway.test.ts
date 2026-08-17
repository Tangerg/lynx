import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  setEmbeddingRole as saveEmbeddingRole,
  setUtilityRole as saveUtilityRole,
  updateProvider,
} from "../application/providerConfig";
import {
  EMBEDDING_ROLE_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "../application/providerQueries";
import type { ProviderConfiguration } from "../application/providerModels";
import { installProviderGateway } from "./runtimeProviderGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
  queryClient.removeQueries({ queryKey: [UTILITY_ROLE_KEY] });
  queryClient.removeQueries({ queryKey: [EMBEDDING_ROLE_KEY] });
});

describe("runtimeProviderGateway", () => {
  it("maps the authoritative provider returned by Runtime", async () => {
    const update = vi.fn().mockResolvedValue({
      id: "openai-compatible",
      baseUrl: "https://models.example.test/v1",
      apiKeyMasked: "sk****st",
      keySource: "stored",
      requiresBaseUrl: true,
      embeddingCapable: true,
      defaultEmbeddingModel: "embed-1",
    });
    setContainer({
      client: () => ({ providers: { update } }) as unknown as LyraClient,
    });
    uninstall = installProviderGateway().dispose;

    await expect(
      updateProvider({
        provider: "openai-compatible",
        apiKey: "sk-test",
        baseUrl: "https://models.example.test/v1",
      }),
    ).resolves.toEqual({
      id: "openai-compatible",
      baseUrl: "https://models.example.test/v1",
      apiKeyMasked: "sk****st",
      keySource: "stored",
      requiresBaseUrl: true,
      embeddingCapable: true,
      defaultEmbeddingModel: "embed-1",
    });
  });

  it("preserves the stored utility and embedding roles", async () => {
    const setUtilityRole = vi.fn().mockResolvedValue({ provider: "openai", model: "chat-1" });
    const setEmbeddingRole = vi.fn().mockResolvedValue({ provider: "openai", model: "embed-1" });
    setContainer({
      client: () => ({ models: { setUtilityRole, setEmbeddingRole } }) as unknown as LyraClient,
    });
    uninstall = installProviderGateway().dispose;

    await expect(saveUtilityRole({ provider: "openai", model: "chat-1" })).resolves.toEqual({
      ok: true,
    });
    await expect(saveEmbeddingRole({ provider: "openai", model: "embed-1" })).resolves.toEqual({
      ok: true,
    });
    expect(queryClient.getQueryData([UTILITY_ROLE_KEY])).toEqual({
      provider: "openai",
      model: "chat-1",
    });
    expect(queryClient.getQueryData([EMBEDDING_ROLE_KEY])).toEqual({
      provider: "openai",
      model: "embed-1",
    });
  });

  it("retires in-flight and queued provider commands before installing a successor", async () => {
    const retiredUpdate = deferred<ProviderConfiguration>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi.fn().mockResolvedValue(
      provider({
        baseUrl: "https://successor.example.test/v1",
        apiKeyMasked: "successor****key",
      }),
    );
    setContainer({
      client: () => ({ providers: { update: updateRetired } }) as unknown as LyraClient,
    });
    const retiredInstallation = installProviderGateway();
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const inFlight = updateProvider({
      provider: "openai-compatible",
      baseUrl: "https://retired.example.test/v1",
    });
    const queued = updateProvider({
      provider: "openai-compatible",
      baseUrl: "https://queued.example.test/v1",
    });
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ providers: { update: updateSuccessor } }) as unknown as LyraClient,
    });
    const successorInstallation = installProviderGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData(
      [PROVIDERS_KEY],
      [
        provider({
          baseUrl: "https://successor.example.test/v1",
          apiKeyMasked: "successor****key",
        }),
      ],
    );

    retiredUpdate.resolve(
      provider({ baseUrl: "https://retired.example.test/v1", apiKeyMasked: "retired****key" }),
    );
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([
      provider({
        baseUrl: "https://successor.example.test/v1",
        apiKeyMasked: "successor****key",
      }),
    ]);

    const successorCommand = updateProvider({
      provider: "openai-compatible",
      baseUrl: "https://successor.example.test/v1",
    });
    retiredInstallation.replaceRuntimeGeneration();
    await expect(successorCommand).resolves.toMatchObject({
      baseUrl: "https://successor.example.test/v1",
    });
    expect(updateSuccessor).toHaveBeenCalledOnce();
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
