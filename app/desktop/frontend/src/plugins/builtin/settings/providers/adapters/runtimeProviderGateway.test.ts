import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { providerGateway } from "../application/ports/providerGateway";
import { installProviderGateway } from "./runtimeProviderGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
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
    uninstall = installProviderGateway();

    await expect(
      providerGateway().updateProvider({
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
    uninstall = installProviderGateway();

    await expect(
      providerGateway().setUtilityRole({ provider: "openai", model: "chat-1" }),
    ).resolves.toEqual({ provider: "openai", model: "chat-1" });
    await expect(
      providerGateway().setEmbeddingRole({ provider: "openai", model: "embed-1" }),
    ).resolves.toEqual({ provider: "openai", model: "embed-1" });
  });
});
