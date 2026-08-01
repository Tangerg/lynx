import { describe, expect, it } from "vitest";

import { isErrorType, RpcError } from "./errors";

describe("typed RPC problems", () => {
  it("narrows structured problem fields by their symbolic type", () => {
    const error = new RpcError({
      code: -32015,
      message: "capability_not_negotiated",
      data: {
        type: "capability_not_negotiated",
        requiredCapabilities: [{ type: "feature", name: "subagents" }],
      },
    });

    expect(isErrorType(error, "capability_not_negotiated")).toBe(true);
    if (!isErrorType(error, "capability_not_negotiated")) {
      throw new Error("expected a capability problem");
    }
    // This access is the compile-time half of the test: the type guard narrows
    // ProblemData to the variant that requires this field.
    expect(error.data.requiredCapabilities).toEqual([{ type: "feature", name: "subagents" }]);
  });

  it("does not classify unrelated problems by code or message", () => {
    const error = new RpcError({
      code: -32015,
      message: "capability_not_negotiated",
      data: { type: "session_busy" },
    });

    expect(isErrorType(error, "capability_not_negotiated")).toBe(false);
    expect(isErrorType(error, "session_busy")).toBe(true);
  });

  it("preserves the typed plugin extension branch", () => {
    const error = new RpcError({
      message: "plugin quota",
      data: { type: "plugin:acme/quota", retryAfterSeconds: 10 },
    });

    if (!isErrorType(error, "plugin:acme/quota")) {
      throw new Error("expected the namespaced plugin problem");
    }
    expect(error.data.retryAfterSeconds).toBe(10);
  });
});
