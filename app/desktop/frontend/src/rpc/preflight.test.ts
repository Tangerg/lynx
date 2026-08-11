import { describe, expect, it } from "vitest";

import { unnegotiated } from "./preflight";
import type { ServerCapabilities } from "./wire.generated";

// These are the runtime's own capability-gate cases, asked of the client's matcher:
// dispatch/contract_test.go pins the same seven requests against the same rules.
// Contract §11.1 requires the dispatcher, discovery and the SDK to consume ONE rule
// set — the table is shared by generation, so what is left to prove is that both
// sides read a rule's condition the same way. A client that judged `watches: []` a
// watch would refuse a call the runtime allows, and no artifact comparison would
// notice.

function advertising(
  features: Record<string, boolean>,
  clientOptIn: string[] = [],
): ServerCapabilities {
  return {
    runEvents: [],
    runtimeTopics: [],
    stateSnapshots: [],
    streamingMethods: [],
    features: Object.fromEntries(
      Object.entries(features).map(([name, enabled]) => [
        name,
        {
          enabled,
          stability: "stable" as const,
          clientOptIn: clientOptIn.includes(name),
          requiredByRunProtocol: false,
        },
      ]),
    ),
    limits: {
      idempotency: { retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    },
  };
}

describe("the capability preflight", () => {
  it("refuses a gated method the server says it cannot do", () => {
    expect(unnegotiated("knowledge.list", {}, advertising({ knowledge: false }))).toEqual([
      "knowledge",
    ]);
    expect(unnegotiated("knowledge.list", {}, advertising({ knowledge: true }))).toEqual([]);
  });

  // §9: a client reads a key the server never advertised as off, which is the same
  // reading the dispatcher applies to its own advertised map.
  it("reads an unadvertised key as off", () => {
    expect(unnegotiated("knowledge.list", {}, advertising({}))).toEqual(["knowledge"]);
  });

  // Refusing on a guess would take away a feature the server offers.
  it("allows everything until something has been negotiated", () => {
    expect(unnegotiated("knowledge.list", {}, null)).toEqual([]);
    expect(unnegotiated("knowledge.list", {}, undefined)).toEqual([]);
  });

  it("leaves an ungated method alone", () => {
    expect(unnegotiated("sessions.list", {}, advertising({}))).toEqual([]);
  });

  it("requires the request to opt into a clientOptIn feature", () => {
    const subagents = advertising({ subagents: true }, ["subagents"]);
    const params = { includeDescendants: true };

    expect(unnegotiated("runs.list", params, subagents)).toEqual(["subagents"]);
    expect(
      unnegotiated("runs.list", params, subagents, {
        features: { subagents: { enabled: true } },
      }),
    ).toEqual([]);
  });

  describe("a conditional rule only bites the gated request", () => {
    const noWatch = advertising({ fileWatch: false });
    const noCheckpoints = advertising({ checkpoints: false });

    it("subscribing without watches needs no fileWatch", () => {
      expect(unnegotiated("runtime.subscribe", {}, noWatch)).toEqual([]);
    });

    it("an empty watch list is not a watch", () => {
      expect(unnegotiated("runtime.subscribe", { watches: [] }, noWatch)).toEqual([]);
    });

    it("registering a watch needs fileWatch", () => {
      expect(unnegotiated("runtime.subscribe", { watches: [{ watchId: "w1" }] }, noWatch)).toEqual([
        "fileWatch",
      ]);
    });

    it("a history rollback needs no checkpoints", () => {
      expect(unnegotiated("sessions.rollback", { sessionId: "ses_1" }, noCheckpoints)).toEqual([]);
    });

    it("restoring files needs checkpoints", () => {
      expect(
        unnegotiated(
          "sessions.rollback",
          { sessionId: "ses_1", restoreType: "files" },
          noCheckpoints,
        ),
      ).toEqual(["checkpoints"]);
    });

    it("restoring both needs checkpoints", () => {
      expect(
        unnegotiated(
          "sessions.rollback",
          { sessionId: "ses_1", restoreType: "both" },
          noCheckpoints,
        ),
      ).toEqual(["checkpoints"]);
    });

    it("restoring files is allowed once checkpoints are on", () => {
      expect(
        unnegotiated(
          "sessions.rollback",
          { sessionId: "ses_1", restoreType: "files" },
          advertising({ checkpoints: true }),
        ),
      ).toEqual([]);
    });
  });
});
