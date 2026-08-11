import { describe, expect, it } from "vitest";
import { codebaseStatusRefreshInterval, type CodebaseStatusReadModel } from "./providerQueries";

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
