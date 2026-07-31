import { describe, expect, it } from "vitest";

import { CLIENT_CAPABILITIES, runtimeRequestMeta } from "./runtimeProtocol";

describe("first-party runtime protocol profile", () => {
  it("opts every request into durable Run trees", () => {
    expect(CLIENT_CAPABILITIES.features?.subagents).toEqual({ enabled: true });
    expect(runtimeRequestMeta().clientCapabilities?.features?.subagents).toEqual({
      enabled: true,
    });
  });
});
