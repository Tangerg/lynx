import { describe, expect, it } from "vitest";
import {
  initialProviderCredentialsDraft,
  providerCredentialsDirty,
  providerCredentialsInput,
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

  it("builds provider configure input from trimmed draft values", () => {
    expect(
      providerCredentialsInput(
        { id: "openai" },
        { apiKey: " sk-test ", baseUrl: "https://gateway.example.test" },
      ),
    ).toEqual({
      provider: "openai",
      apiKey: "sk-test",
      baseUrl: "https://gateway.example.test",
    });
  });
});
