import { describe, expect, it } from "vitest";
import {
  initialProviderCredentialsDraft,
  providerCredentialsDirty,
  providerCredentialsInput,
  providerCredentialsValid,
} from "./providerDraft";

describe("providerDraft", () => {
  it("initializes from persisted provider settings without copying secrets", () => {
    expect(initialProviderCredentialsDraft({ baseUrl: "https://api.example.test" })).toEqual({
      apiKey: "",
      baseUrl: "https://api.example.test",
    });
  });

  it("tracks credential changes", () => {
    const provider = { baseUrl: "https://api.example.test" };

    expect(providerCredentialsDirty(provider, { apiKey: "", baseUrl: provider.baseUrl })).toBe(
      false,
    );
    expect(providerCredentialsDirty(provider, { apiKey: " key ", baseUrl: provider.baseUrl })).toBe(
      true,
    );
    expect(providerCredentialsDirty(provider, { apiKey: "", baseUrl: "" })).toBe(true);
  });

  it("builds explicit provider changes from trimmed draft values", () => {
    expect(
      providerCredentialsInput(
        { id: "openai", baseUrl: "" },
        { apiKey: " sk-test ", baseUrl: "https://gateway.example.test" },
      ),
    ).toEqual({
      provider: "openai",
      apiKey: "sk-test",
      baseUrl: "https://gateway.example.test",
    });
  });

  it("preserves an untouched secret and explicitly clears an edited endpoint", () => {
    expect(
      providerCredentialsInput(
        { id: "openai", baseUrl: "https://gateway.example.test" },
        { apiKey: "", baseUrl: "" },
      ),
    ).toEqual({ provider: "openai", baseUrl: null });
  });

  it("requires a non-blank endpoint only for endpoint-defined providers", () => {
    expect(
      providerCredentialsValid({ requiresBaseUrl: true }, { apiKey: "key", baseUrl: "   " }),
    ).toBe(false);
    expect(
      providerCredentialsValid(
        { requiresBaseUrl: true },
        {
          apiKey: "key",
          baseUrl: " https://models.example.test/v1 ",
        },
      ),
    ).toBe(true);
    expect(
      providerCredentialsValid({ requiresBaseUrl: false }, { apiKey: "key", baseUrl: "" }),
    ).toBe(true);
  });

  it("trims an edited endpoint before persistence", () => {
    expect(
      providerCredentialsInput(
        { id: "openai-compatible", baseUrl: "" },
        { apiKey: "", baseUrl: " https://models.example.test/v1 " },
      ),
    ).toEqual({
      provider: "openai-compatible",
      baseUrl: "https://models.example.test/v1",
    });
  });
});
