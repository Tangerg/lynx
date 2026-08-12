import { describe, expect, it } from "vitest";
import {
  codebaseStatusRefreshInterval,
  providerRoleIsAvailable,
  type CodebaseStatusReadModel,
} from "./providerQueries";

function status(overrides: Partial<CodebaseStatusReadModel> = {}): CodebaseStatusReadModel {
  return {
    state: "ready",
    fileCount: 12,
    chunkCount: 34,
    ...overrides,
  };
}

describe("codebaseStatusRefreshInterval", () => {
  it("refreshes while Runtime owns an active reindex operation", () => {
    expect(
      codebaseStatusRefreshInterval(status({ state: "indexing", operationId: "op_123" })),
    ).toBe(1_000);
  });

  it.each([undefined, status(), status({ state: "error" }), status({ state: "indexing" })])(
    "stops without an active Runtime operation",
    (value) => {
      expect(codebaseStatusRefreshInterval(value)).toBe(false);
    },
  );
});

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
