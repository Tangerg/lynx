import { describe, expect, it } from "vitest";
import { providerRoleIsAvailable } from "./providerQueries";

describe("providerRoleIsAvailable", () => {
  const providers = [
    { id: "configured", baseUrl: "", apiKeyMasked: "sk****42" },
    { id: "missing-key", baseUrl: "", apiKeyMasked: "" },
  ];

  it("requires both a complete role and a currently configured provider", () => {
    expect(providerRoleIsAvailable({ provider: "configured", model: "model-1" }, providers)).toBe(
      true,
    );
    expect(providerRoleIsAvailable({ provider: "missing-key", model: "model-1" }, providers)).toBe(
      false,
    );
    expect(providerRoleIsAvailable({ provider: "configured" }, providers)).toBe(false);
    expect(providerRoleIsAvailable(undefined, providers)).toBe(false);
  });

  it("does not treat a role for an absent provider as executable", () => {
    expect(providerRoleIsAvailable({ provider: "removed", model: "model-1" }, providers)).toBe(
      false,
    );
  });
});
