import { readFileSync } from "node:fs";
import { join } from "node:path";

import Ajv2020 from "ajv/dist/2020";
import { describe, expect, it } from "vitest";

import { WIRE_SAMPLES } from "./wire.samples.generated";

// The PUBLISHED schema, checked by a validator that had no hand in producing it.
//
// This is the third leg contract §11.3 asks for, and the only one that can catch a
// bug in the compiler behind the other two: the TypeScript types and the runtime
// checks are both derived from the same schema tree, so a mistake in that derivation
// makes them agree with each other and with nothing else. An off-the-shelf JSON
// Schema implementation reading contract/schema.json answers the question a third
// party actually asks — does the document the runtime publishes accept the frames the
// runtime produces?
//
// It reads the bundle from the runtime's tree rather than holding a copy. The Go
// round-trip already reads this repository's samples across the same boundary, for
// the same reason: an artifact with two homes has two versions.
const CONTRACT = join(import.meta.dirname, "../../../../runtime/contract");
const SAMPLES = join(import.meta.dirname, "samples");

function read(path: string): unknown {
  return JSON.parse(readFileSync(path, "utf8")) as unknown;
}

// strict mode off: it rejects a schema for style reasons this bundle does not owe it
// — an `enum` beside its `type`, a `$defs` entry nothing in the document references
// (gate 4 already proves every definition IS referenced, from the method graph the
// bundle does not itself describe).
const ajv = new Ajv2020({ strict: false, allErrors: true });
const bundle = read(join(CONTRACT, "schema.json")) as { $defs: Record<string, unknown> };
ajv.addSchema(bundle, "schema.json");

describe("the published JSON Schema bundle", () => {
  it("compiles", () => {
    expect(Object.keys(bundle.$defs).length).toBeGreaterThan(0);
    for (const name of Object.keys(bundle.$defs)) {
      expect(() => ajv.getSchema(`schema.json#/$defs/${name}`)).not.toThrow();
    }
  });

  it.each(WIRE_SAMPLES)("accepts $file as $shape", ({ file, shape }) => {
    const validate = ajv.getSchema(`schema.json#/$defs/${shape}`);
    if (!validate) throw new Error(`the bundle defines no ${shape}`);
    validate(read(join(SAMPLES, file)));
    expect(validate.errors ?? []).toEqual([]);
  });

  // Every fixture above is a VALID frame, so on its own this suite would pass just as
  // well against a schema that accepted anything. These two say it does not: one
  // missing required field, and one variant carrying another variant's.
  it("refuses a frame the runtime would not produce", () => {
    const session = ajv.getSchema("schema.json#/$defs/Session");
    expect(session?.({ title: "no id" })).toBe(false);

    const block = ajv.getSchema("schema.json#/$defs/ContentBlock");
    expect(block?.({ type: "text", text: "hello" })).toBe(true);
    expect(block?.({ type: "text", text: "hello", mime: "image/png" })).toBe(false);

    const cancel = ajv.getSchema("schema.json#/$defs/CancelRunResponse");
    const canceledRun = {
      id: "run_01",
      sessionId: "ses_01",
      status: "finished",
      outcome: { type: "canceled" },
      finishedAt: "2026-07-30T00:00:00Z",
      metrics: { steps: 0, activeDurationMs: 0 },
      protocolProfile: { requiredFeatures: [], interruptTypes: [] },
    };
    expect(cancel?.({ type: "root", run: canceledRun })).toBe(true);
    expect(cancel?.({ type: "root", run: canceledRun, rootRun: canceledRun })).toBe(false);
    expect(cancel?.({ type: "child", run: canceledRun })).toBe(false);

    const steer = ajv.getSchema("schema.json#/$defs/SteerRunRequest");
    expect(
      steer?.({
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [
          { type: "text", text: "compare this" },
          { type: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    ).toBe(true);
    expect(steer?.({ runId: "run_01", expectedSegmentId: "seg_01", input: [] })).toBe(false);
    expect(steer?.({ runId: "run_01", expectedSegmentId: "seg_01", message: "legacy" })).toBe(
      false,
    );

    const streamEvent = ajv.getSchema("schema.json#/$defs/StreamEvent");
    expect(streamEvent?.({ type: "custom", name: "vendor.preview" })).toBe(true);
    expect(streamEvent?.({ type: "custom", name: "vendor.preview", durable: true })).toBe(false);

    const clientCapabilities = ajv.getSchema("schema.json#/$defs/ClientCapabilities");
    expect(
      clientCapabilities?.({
        excludedEphemeralEvents: ["segment.progress", "item.delta"],
      }),
    ).toBe(true);
    expect(clientCapabilities?.({ excludedEphemeralEvents: ["custom"] })).toBe(false);
    expect(clientCapabilities?.({ excludedEphemeralEvents: ["item.completed"] })).toBe(false);

    const runtimeEvent = ajv.getSchema("schema.json#/$defs/RuntimeEvent");
    expect(runtimeEvent?.({ type: "files.changed", sequence: 1, paths: ["README.md"] })).toBe(true);
    expect(runtimeEvent?.({ type: "skills.changed", sequence: 0 })).toBe(false);
    expect(runtimeEvent?.({ type: "files.changed", sequence: 1, paths: [] })).toBe(false);
    expect(runtimeEvent?.({ type: "resync", sequence: 1 })).toBe(false);
    expect(runtimeEvent?.({ type: "resync", sequence: 1, topics: [] })).toBe(false);
    expect(runtimeEvent?.({ type: "sessions.changed", sequence: 1, sessionIds: [] })).toBe(false);

    const runtimeLimits = ajv.getSchema("schema.json#/$defs/RuntimeLimits");
    expect(
      runtimeLimits?.({
        idempotency: { retentionSeconds: 86_400 },
        runReplay: {
          scope: "processRootSegment",
          maxEvents: 2048,
          maxBytes: 16_777_216,
        },
        runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
      }),
    ).toBe(true);
    expect(
      runtimeLimits?.({
        runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
      }),
    ).toBe(false);

    const pendingInterruptSet = ajv.getSchema("schema.json#/$defs/PendingInterruptSet");
    expect(
      pendingInterruptSet?.({
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toBe(false);

    const problem = ajv.getSchema("schema.json#/$defs/ProblemData");
    expect(
      problem?.({
        type: "capability_not_negotiated",
        requiredCapabilities: [],
      }),
    ).toBe(false);
    const repeatedRequirement = { type: "feature", name: "subagents" };
    expect(
      problem?.({
        type: "capability_not_negotiated",
        requiredCapabilities: [repeatedRequirement, repeatedRequirement],
      }),
    ).toBe(false);
    expect(problem?.({ type: "run_lost" })).toBe(true);
    expect(problem?.({ type: "mcp_dial_failed", detail: "connection failed" })).toBe(false);
    expect(problem?.({ type: "plugin:acme/model_timeout", retryAfterSeconds: 2 })).toBe(true);
    expect(problem?.({ type: "model_timeout" })).toBe(false);
    expect(problem?.({ type: "plugin:Acme/model_timeout" })).toBe(false);
    expect(
      problem?.({
        type: "run_lost",
        activeRun: { runId: "run_1", status: "running" },
      }),
    ).toBe(false);
    expect(problem?.({ type: "idempotency_in_progress", retryAfterSeconds: 0 })).toBe(false);
  });
});
