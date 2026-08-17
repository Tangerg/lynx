import { describe, expect, it } from "vitest";
import type { RuntimeServiceSnapshot } from "./ports/serviceStatus";
import { runtimeServiceAcceptsCommands } from "./runtimeCommandAvailability";

const observation = {
  server: { name: "lyra", version: "test" },
  protocol: { current: "2", minSupported: "2" },
  health: "ready" as const,
  checks: {},
};

describe("runtime command availability", () => {
  it("rejects commands before the first inspection and after a disconnect", () => {
    for (const snapshot of [
      { phase: "checking", observation: null, failure: null },
      { phase: "reconnecting", observation: null, failure: null },
      {
        phase: "unavailable",
        observation: null,
        failure: { reason: "failed", detail: "connection refused" },
      },
    ] satisfies RuntimeServiceSnapshot[]) {
      expect(runtimeServiceAcceptsCommands(snapshot)).toBe(false);
    }
  });

  it("keeps commands available during a refresh of a proven connection", () => {
    expect(runtimeServiceAcceptsCommands({ phase: "checking", observation, failure: null })).toBe(
      true,
    );
  });

  it("accepts commands whenever the inspected service remains connected", () => {
    for (const phase of ["ready", "degraded", "unhealthy"] as const) {
      expect(runtimeServiceAcceptsCommands({ phase, observation, failure: null })).toBe(true);
    }
  });
});
