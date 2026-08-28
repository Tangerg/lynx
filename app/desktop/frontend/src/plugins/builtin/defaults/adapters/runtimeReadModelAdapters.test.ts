import { describe, expect, it } from "vitest";
import type { Session } from "@/rpc";
import { toAgentSessionSummary } from "./runtimeReadModelAdapters";

function runtimeSession(availability: "available" | "missing"): Session {
  return {
    id: "ses_1",
    title: "Session",
    status: "idle",
    provider: "provider",
    model: "model",
    reasoningEffort: "high",
    workspace: {
      ref: { path: "/repo" },
      projectRoot: "/repo",
      availability,
    },
    createdAt: "2026-08-24T00:00:00Z",
    updatedAt: "2026-08-24T00:00:00Z",
    revision: 1,
  };
}

describe("Runtime read-model adapters", () => {
  it("keeps workspace identity and availability in one consumer-owned value", () => {
    const summary = toAgentSessionSummary(runtimeSession("missing"));
    expect(summary.workspace).toEqual({
      path: "/repo",
      availability: "missing",
    });
    expect(summary.reasoningEffort).toBe("high");
  });
});
