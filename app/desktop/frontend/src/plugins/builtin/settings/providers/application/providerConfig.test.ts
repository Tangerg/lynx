import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { EMBEDDING_ROLE_KEY, PROVIDERS_KEY, UTILITY_ROLE_KEY } from "./providerQueries";
import { setEmbeddingRole, setUtilityRole, updateProvider } from "./providerConfig";
import type { ProviderGateway } from "./ports/providerGateway";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";
import { ProviderMutationOwner } from "./providerMutationOwner";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

let uninstall: (() => void) | undefined;

function installGateway(gateway: ProviderGateway): void {
  const owner = ProviderMutationOwner.install(gateway);
  uninstall = () => owner.dispose();
}

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
  queryClient.removeQueries({ queryKey: [UTILITY_ROLE_KEY] });
  queryClient.removeQueries({ queryKey: [EMBEDDING_ROLE_KEY] });
});

describe("provider configuration", () => {
  it("commits the authoritative provider response", async () => {
    queryClient.setQueryData(
      [PROVIDERS_KEY],
      [
        { id: "openai", baseUrl: "", apiKeyMasked: "" },
        { id: "deepseek", baseUrl: "", apiKeyMasked: "ds****st" },
      ],
    );
    const saved = {
      id: "openai",
      baseUrl: "https://models.example.test/v1",
      apiKeyMasked: "sk****st",
      keySource: "stored" as const,
      embeddingCapable: true,
      defaultEmbeddingModel: "embed-1",
    };
    installGateway({
      updateProvider: vi.fn().mockResolvedValue(saved),
    } as unknown as ProviderGateway);

    await expect(updateProvider({ provider: "openai", baseUrl: saved.baseUrl })).resolves.toEqual(
      saved,
    );
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([
      saved,
      { id: "deepseek", baseUrl: "", apiKeyMasked: "ds****st" },
    ]);
  });

  it("serializes changes to one role and continues after a rejected change", async () => {
    const first = deferred<ProviderRole>();
    const second = deferred<ProviderRole>();
    const setUtilityRoleGateway = vi
      .fn<(role: ProviderRole) => Promise<ProviderRole>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    installGateway({
      setUtilityRole: setUtilityRoleGateway,
      errorMessage: () => undefined,
    } as unknown as ProviderGateway);

    const rejected = setUtilityRole({ provider: "openai", model: "first" });
    const accepted = setUtilityRole({ provider: "openai", model: "second" });
    await vi.waitFor(() => expect(setUtilityRoleGateway).toHaveBeenCalledTimes(1));

    first.reject(new Error("invalid first role"));
    await expect(rejected).resolves.toEqual({ ok: false, error: "invalid first role" });
    await vi.waitFor(() =>
      expect(setUtilityRoleGateway).toHaveBeenNthCalledWith(2, {
        provider: "openai",
        model: "second",
      }),
    );

    second.resolve({ provider: "openai", model: "second" });
    await expect(accepted).resolves.toEqual({ ok: true });
    expect(queryClient.getQueryData([UTILITY_ROLE_KEY])).toEqual({
      provider: "openai",
      model: "second",
    });
  });

  it("keeps utility and embedding role resources independent", async () => {
    const utility = deferred<ProviderRole>();
    const embedding = deferred<ProviderRole>();
    const setUtilityRoleGateway = vi.fn().mockReturnValue(utility.promise);
    const setEmbeddingRoleGateway = vi.fn().mockReturnValue(embedding.promise);
    installGateway({
      setUtilityRole: setUtilityRoleGateway,
      setEmbeddingRole: setEmbeddingRoleGateway,
    } as unknown as ProviderGateway);

    const utilityResult = setUtilityRole({ provider: "openai", model: "chat-1" });
    const embeddingResult = setEmbeddingRole({ provider: "openai", model: "embed-1" });
    await vi.waitFor(() => {
      expect(setUtilityRoleGateway).toHaveBeenCalledTimes(1);
      expect(setEmbeddingRoleGateway).toHaveBeenCalledTimes(1);
    });

    utility.resolve({ provider: "openai", model: "chat-1" });
    embedding.resolve({ provider: "openai", model: "embed-1" });
    await expect(utilityResult).resolves.toEqual({ ok: true });
    await expect(embeddingResult).resolves.toEqual({ ok: true });
  });

  it("continues provider changes after a rejected command", async () => {
    const first = deferred<ProviderConfiguration>();
    const saved = {
      id: "deepseek",
      baseUrl: "https://api.deepseek.test",
      apiKeyMasked: "ds****st",
    };
    const updateProviderGateway = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(saved);
    installGateway({
      updateProvider: updateProviderGateway,
    } as unknown as ProviderGateway);

    const rejected = updateProvider({ provider: "openai", baseUrl: "https://invalid.test" });
    const accepted = updateProvider({ provider: "deepseek", baseUrl: saved.baseUrl });
    first.reject(new Error("not saved"));

    await expect(rejected).rejects.toThrow("not saved");
    await expect(accepted).resolves.toEqual(saved);
    expect(updateProviderGateway).toHaveBeenNthCalledWith(2, {
      provider: "deepseek",
      baseUrl: saved.baseUrl,
    });
  });
});
