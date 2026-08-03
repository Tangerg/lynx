import { describe, expect, it } from "vitest";
import { needsProviderSetup } from "./providerSetup";

describe("needsProviderSetup", () => {
  it("does not show setup while providers are still loading", () => {
    expect(needsProviderSetup(undefined)).toBe(false);
  });

  it("shows setup only when every provider lacks a saved key", () => {
    expect(needsProviderSetup([{ apiKeyMasked: "" }])).toBe(true);
    expect(needsProviderSetup([{ apiKeyMasked: "" }, { apiKeyMasked: "sk-..." }])).toBe(false);
  });
});
