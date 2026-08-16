import { describe, expect, it } from "vitest";

import {
  validateMethodResult,
  validateNotificationParams,
  validateWire,
} from "./wire.validate.generated";
import { runEventReliability } from "./wire.generated";

// One case per rule the compiler translates, because each rule takes its own code
// path out of the schema tree: a type keyword, `required`, a closed `enum`, a value
// constraint stated beside a type and again alone in an allOf branch, union
// exclusivity, and a cross-field presence rule. A validator that agreed with the
// published schema on nine of them and silently dropped the tenth would still pass
// every canonical sample — the samples are valid frames, so only an invalid one
// proves a rule is enforced.

const session = {
  id: "ses_01",
  title: "Refactor the runtime protocol",
  status: "idle",
  model: "claude-opus-4-8",
  workspace: {
    ref: { path: "/Users/dev/project" },
    projectRoot: "/Users/dev/project",
    availability: "available",
  },
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
  revision: 3,
};

const artifactSession = {
  id: "ses_01",
  title: "Refactor the runtime protocol",
  model: "claude-opus-4-8",
  workspace: { path: "/Users/dev/project" },
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
};

const finishedRun = {
  id: "run_01",
  sessionId: "ses_01",
  status: "finished",
  createdAt: "2026-07-07T10:00:00Z",
  finishedAt: "2026-07-07T10:00:20Z",
  metrics: { steps: 2, activeDurationMillis: 20_000 },
  protocolProfile: { requiredFeatures: [], interruptTypes: [] },
  outcome: { type: "completed" },
};

describe("the generated wire checks", () => {
  it("derives run-event reliability from the protocol event registry", () => {
    expect(runEventReliability("segment.finished")).toBe("authoritative");
    expect(runEventReliability("item.delta")).toBe("ephemeral");
    expect(runEventReliability("future.event")).toBeUndefined();
  });

  it("accepts a well-formed frame", () => {
    expect(validateWire("Session", session)).toEqual([]);
  });

  it("binds every method result to its registered wire shape", () => {
    expect(validateMethodResult("sessions.get", session)).toEqual([]);
    const { revision: _revision, ...malformed } = session;
    expect(validateMethodResult("sessions.get", malformed)).toEqual([
      { path: "sessions.get.result.revision", detail: "is required" },
    ]);
    expect(validateMethodResult("goals.get", null)).toEqual([]);
    expect(validateMethodResult("sessions.delete", {})).toEqual([]);
  });

  it("binds every downstream notification to its registered params shape", () => {
    expect(
      validateNotificationParams("notifications.runtime.event", {
        event: { type: "skills.changed", sequence: 1 },
      }),
    ).toEqual([]);
    expect(
      validateNotificationParams("notifications.runtime.event", {
        event: { type: "skills.changed", sequence: 0 },
      }),
    ).toContainEqual({
      path: "notifications.runtime.event.params.event.sequence",
      detail: "expected at least 1",
    });
  });

  it("names every missing required field", () => {
    const { id: _id, revision: _revision, ...partial } = session;
    expect(validateWire("Session", partial)).toEqual([
      { path: "Session.id", detail: "is required" },
      { path: "Session.revision", detail: "is required" },
    ]);
  });

  it("keeps start and resume acknowledgements semantically distinct", () => {
    expect(
      validateWire("StartRunResponse", {
        runId: "run_01",
        segmentId: "seg_01",
        userItemId: "item_01",
      }),
    ).toEqual([]);
    expect(validateWire("StartRunResponse", { runId: "run_01", segmentId: "seg_01" })).toEqual([
      { path: "StartRunResponse.userItemId", detail: "is required" },
    ]);
    expect(validateWire("ResumeRunResponse", { runId: "run_01", segmentId: "seg_02" })).toEqual([]);
    expect(
      validateWire("ResumeRunResponse", {
        runId: "run_01",
        segmentId: "seg_02",
        userItemId: "",
      }),
    ).toEqual([
      {
        path: "ResumeRunResponse.userItemId",
        detail: "expected at least 1 character(s)",
      },
    ]);
  });

  it("rejects a value of the wrong JSON type", () => {
    expect(validateWire("Session", { ...session, revision: "3" })).toEqual([
      { path: "Session.revision", detail: "expected an integer" },
    ]);
  });

  it("rejects a tag outside a closed value set", () => {
    const [violation] = validateWire("Session", { ...session, status: "sleeping" });
    expect(violation?.path).toBe("Session.status");
    expect(violation?.detail).toContain("expected one of");
  });

  // Shared result definitions stay open so an older client tolerates optional
  // fields added by a newer runtime. Request strictness is stated contextually by
  // OpenRPC and enforced by the runtime's request decoder.
  it("ignores a property the contract does not mention", () => {
    expect(validateWire("Session", { ...session, inventedByANewerServer: true })).toEqual([]);
  });

  it("rejects an empty string where the contract states a minimum length", () => {
    expect(validateWire("GetSessionRequest", { sessionId: "ses_01" })).toEqual([]);
    expect(validateWire("GetSessionRequest", { sessionId: "" })).toEqual([
      { path: "GetSessionRequest.sessionId", detail: "expected at least 1 character(s)" },
    ]);
  });

  // An omitted filter already means "every status", so the two ways of sending one
  // that means nothing — empty, or repeating a value — are the ones refused.
  it("rejects a filter array that is empty or repeats a value", () => {
    expect(validateWire("ListRunsRequest", {})).toEqual([]);
    expect(validateWire("ListRunsRequest", { statuses: ["running", "waiting"] })).toEqual([]);
    expect(validateWire("ListRunsRequest", { statuses: [] })).toEqual([
      { path: "ListRunsRequest.statuses", detail: "expected at least 1 item(s)" },
    ]);
    expect(validateWire("ListRunsRequest", { statuses: ["running", "running"] })).toEqual([
      { path: "ListRunsRequest.statuses", detail: "expected no repeated items" },
    ]);
  });

  it("rejects an empty secret-map replacement", () => {
    expect(
      validateWire("MCPHeadersChange", { type: "set", value: { "X-API-Key": "secret" } }),
    ).toEqual([]);
    expect(validateWire("MCPHeadersChange", { type: "set", value: {} })).toEqual([
      { path: "MCPHeadersChange.value", detail: "expected at least 1 property" },
    ]);
  });

  it("requires structured non-empty steering input", () => {
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [
          { type: "text", text: "compare this" },
          { type: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    ).toEqual([]);
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        input: [],
      }),
    ).toEqual([{ path: "SteerRunRequest.input", detail: "expected at least 1 item(s)" }]);
    expect(
      validateWire("SteerRunRequest", {
        runId: "run_01",
        expectedSegmentId: "seg_01",
        message: "legacy",
      }),
    ).toEqual([{ path: "SteerRunRequest.input", detail: "is required" }]);
  });

  it("rejects a revision below the minimum", () => {
    expect(
      validateWire("UpdateSessionRequest", { sessionId: "ses_01", expectedRevision: 0 }),
    ).toEqual([{ path: "UpdateSessionRequest.expectedRevision", detail: "expected at least 1" }]);
  });

  it("enforces generated request bounds", () => {
    expect(validateWire("PageQuery", { limit: -1 })).toEqual([
      { path: "PageQuery.limit", detail: "expected at least 0" },
    ]);
    expect(validateWire("GenerationParams", { temperature: 2.1 })).toEqual([
      { path: "GenerationParams.temperature", detail: "expected at most 2" },
    ]);
    expect(validateWire("GenerationParams", { topP: 1.1 })).toEqual([
      { path: "GenerationParams.topP", detail: "expected at most 1" },
    ]);

    const boundaryReason = "😀".repeat(1024);
    expect(validateWire("CancelRunRequest", { runId: "run_01", reason: boundaryReason })).toEqual(
      [],
    );
    expect(
      validateWire("CancelRunRequest", { runId: "run_01", reason: `${boundaryReason}😀` }),
    ).toEqual([
      {
        path: "CancelRunRequest.reason",
        detail: "expected at most 1024 character(s)",
      },
    ]);
  });

  // The constraint belongs to this request, not to every carrier of the shared
  // shape, so the schema states it in an allOf branch — a third code path, and the
  // one that reads `minLength` with no type keyword beside it.
  it("states a constraint on a field of a shared shape", () => {
    const artifact = {
      version: 19,
      session: artifactSession,
      items: [],
      messages: [],
      runs: [],
      toolResults: [],
    };
    expect(validateWire("ImportSessionRequest", { artifact })).toEqual([]);
    expect(
      validateWire("ImportSessionRequest", {
        artifact: { ...artifact, session: { ...artifactSession, id: "" } },
      }),
    ).toEqual([
      {
        path: "ImportSessionRequest.artifact.session.id",
        detail: "expected at least 1 character(s)",
      },
    ]);
  });

  it("accepts one variant of a union and refuses another variant's field", () => {
    expect(validateWire("ContentBlock", { type: "text", text: "hello" })).toEqual([]);
    expect(
      validateWire("ContentBlock", { type: "text", text: "hello", mime: "image/png" }),
    ).toEqual([
      { path: "ContentBlock", detail: "matches no permitted variant" },
      { path: "ContentBlock.mime", detail: "must not be present here" },
    ]);
  });

  it("keeps cancel root and child results closed and distinct", () => {
    const canceledRoot = { ...finishedRun, outcome: { type: "canceled" } };
    const canceledChild = {
      ...canceledRoot,
      id: "run_child",
      spawnedByItemId: "item_parent",
      parentRunId: "run_01",
      rootRunId: "run_01",
    };
    expect(validateWire("CancelRunResponse", { type: "root", run: canceledRoot })).toEqual([]);
    expect(
      validateWire("CancelRunResponse", {
        type: "child",
        run: canceledChild,
        rootRun: canceledRoot,
      }),
    ).toEqual([]);
    expect(
      validateWire("CancelRunResponse", {
        type: "root",
        run: canceledRoot,
        rootRun: canceledRoot,
      }),
    ).toEqual([
      { path: "CancelRunResponse", detail: "matches no permitted variant" },
      { path: "CancelRunResponse.rootRun", detail: "must not be present here" },
    ]);
    expect(validateWire("CancelRunResponse", { type: "child", run: canceledChild })).toEqual([
      { path: "CancelRunResponse", detail: "matches no permitted variant" },
      { path: "CancelRunResponse.rootRun", detail: "is required" },
    ]);
  });

  // The scope of a read is a union for the same reason a content block is: a frame
  // carrying both subjects would need a precedence rule to resolve, and the flag only
  // means something where there is a subtree to include.
  it("keeps a read's two scopes exclusive", () => {
    expect(validateWire("ItemListScope", { type: "session", sessionId: "ses_01" })).toEqual([]);
    expect(
      validateWire("ItemListScope", { type: "run", runId: "run_01", includeDescendants: true }),
    ).toEqual([]);
    expect(
      validateWire("ItemListScope", { type: "session", sessionId: "ses_01", runId: "run_01" }),
    ).toEqual([
      { path: "ItemListScope", detail: "matches no permitted variant" },
      { path: "ItemListScope.runId", detail: "must not be present here" },
    ]);
    expect(
      validateWire("ItemListScope", {
        type: "session",
        sessionId: "ses_01",
        includeDescendants: true,
      }),
    ).toEqual([
      { path: "ItemListScope", detail: "matches no permitted variant" },
      { path: "ItemListScope.includeDescendants", detail: "must not be present here" },
    ]);
  });

  it("refuses a discriminator no variant claims", () => {
    const details = validateWire("ContentBlock", { type: "video", data: "AAAA" }).map(
      (v) => v.detail,
    );
    expect(details).toContain("matches no permitted variant");
  });

  // A rule declared for RunSummary has to reach the RunRef that embeds it: the
  // fields are inlined onto one frame, so a rule that stopped at the summary would
  // leave the shape a client actually receives unchecked.
  it("applies an embedded shape's rules to the shape embedding it", () => {
    const { parentRunId: _parent, ...rootChild } = {
      ...finishedRun,
      spawnedByItemId: "item_03",
      parentRunId: "run_02",
      rootRunId: "run_02",
    };
    // Named once per edge that demands it: the rule is all-or-none stated per edge,
    // so both surviving edges independently require the missing one.
    expect(validateWire("RunRef", rootChild)).toEqual([
      { path: "RunRef.parentRunId", detail: "is required" },
      { path: "RunRef.parentRunId", detail: "is required" },
    ]);
    // The counter-example: all three edges together is what a child looks like.
    expect(validateWire("RunRef", { ...rootChild, parentRunId: "run_02" })).toEqual([]);
  });

  it("enforces a cross-field presence rule", () => {
    expect(validateWire("RunRef", finishedRun)).toEqual([]);
    const { outcome: _outcome, ...unexplained } = finishedRun;
    expect(validateWire("RunRef", unexplained)).toEqual([
      { path: "RunRef.outcome", detail: "is required" },
    ]);
  });

  it("checks array elements and names the index", () => {
    expect(validateWire("PageOfSession", { data: [session, session] })).toEqual([]);
    expect(
      validateWire("PageOfSession", { data: [session, { ...session, revision: "3" }] }),
    ).toEqual([{ path: "PageOfSession.data[1].revision", detail: "expected an integer" }]);
  });

  it("carries any JSON where the contract publishes an opaque passthrough", () => {
    const custom = {
      type: "custom",
      name: "vendor.event",
      payload: { anything: [1, "two", null] },
    };
    expect(validateWire("StreamEvent", custom)).toEqual([]);
    expect(validateWire("StreamEvent", { ...custom, durable: true })).toEqual([
      { path: "StreamEvent", detail: "matches no permitted variant" },
      { path: "StreamEvent.durable", detail: "must not be present here" },
    ]);
  });

  it("keeps the run-event opt-out vocabulary narrower than the event union", () => {
    expect(
      validateWire("ClientCapabilities", {
        excludedEphemeralEvents: ["segment.progress", "item.delta"],
      }),
    ).toEqual([]);
    expect(
      validateWire("ClientCapabilities", {
        excludedEphemeralEvents: ["custom"],
      }),
    ).toEqual([
      {
        path: "ClientCapabilities.excludedEphemeralEvents[0]",
        detail: 'expected one of "segment.progress", "item.delta"',
      },
    ]);
  });

  it("enforces output value constraints from the same machine contract", () => {
    expect(validateWire("RuntimeEvent", { type: "skills.changed", sequence: 0 })).toEqual([
      { path: "RuntimeEvent.sequence", detail: "expected at least 1" },
    ]);
    expect(
      validateWire("RuntimeEvent", {
        type: "files.changed",
        sequence: 1,
        paths: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.paths",
      detail: "expected at least 1 item(s)",
    });
    expect(validateWire("RuntimeEvent", { type: "resync", sequence: 1 })).toContainEqual({
      path: "RuntimeEvent.topics",
      detail: "is required",
    });
    expect(
      validateWire("RuntimeEvent", {
        type: "resync",
        sequence: 1,
        topics: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.topics",
      detail: "expected at least 1 item(s)",
    });
    expect(
      validateWire("RuntimeEvent", {
        type: "sessions.changed",
        sequence: 1,
        sessionIds: [],
      }),
    ).toContainEqual({
      path: "RuntimeEvent.sessionIds",
      detail: "expected at least 1 item(s)",
    });

    expect(
      validateWire("RuntimeLimits", {
        runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
      }),
    ).toEqual([
      { path: "RuntimeLimits.idempotency", detail: "is required" },
      { path: "RuntimeLimits.mcpAuthorizationAttempts", detail: "is required" },
      { path: "RuntimeLimits.runReplay", detail: "is required" },
    ]);
    expect(
      validateWire("IdempotencyLimits", { namespace: "idp_test", retentionSeconds: 0 }),
    ).toEqual([{ path: "IdempotencyLimits.retentionSeconds", detail: "expected at least 1" }]);
    expect(validateWire("IdempotencyLimits", { namespace: "", retentionSeconds: 1 })).toEqual([
      { path: "IdempotencyLimits.namespace", detail: "expected at least 1 character(s)" },
    ]);
    expect(validateWire("MCPAuthorizationAttemptLimits", { retentionSeconds: 0 })).toEqual([
      { path: "MCPAuthorizationAttemptLimits.retentionSeconds", detail: "expected at least 1" },
    ]);
    expect(
      validateWire("PendingInterruptSet", {
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toContainEqual({
      path: "PendingInterruptSet.interrupts",
      detail: "expected at least 1 item(s)",
    });
    expect(
      validateWire("PendingInterruptSet", {
        rootRunId: "run_01",
        sessionId: "ses_01",
        interrupts: [
          {
            type: "question",
            itemId: "item_01",
            runId: "",
            payload: {
              question: { fields: [{ type: "text", prompt: "Continue?" }] },
            },
          },
        ],
        createdAt: "2026-07-30T00:00:00Z",
      }),
    ).toContainEqual({
      path: "PendingInterruptSet.interrupts[0].runId",
      detail: "expected at least 1 character(s)",
    });
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "choice",
            prompt: "Choose",
            options: [{ label: "A" }, { label: "B" }],
            allowCustom: true,
          },
        ],
      }),
    ).toEqual([]);
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "choice",
            prompt: "Choose",
            header: "😀".repeat(12),
            options: [{ label: "A" }],
          },
        ],
      }),
    ).toContainEqual({
      path: "Question.fields[0].options",
      detail: "expected at least 2 item(s)",
    });
    expect(
      validateWire("Question", {
        fields: [
          {
            type: "text",
            prompt: "Explain",
            header: "😀".repeat(13),
          },
        ],
      }),
    ).toContainEqual({
      path: "Question.fields[0].header",
      detail: "expected at most 12 character(s)",
    });
    expect(
      validateWire("InterruptResponseValue", {
        type: "answer",
        answers: { q0: ["A"] },
      }),
    ).toContainEqual({
      path: "InterruptResponseValue.answers",
      detail: "expected an array",
    });
    expect(
      validateWire("ProblemData", {
        type: "capability_not_negotiated",
        requiredCapabilities: [],
      }),
    ).toContainEqual({
      path: "ProblemData.requiredCapabilities",
      detail: "expected at least 1 item(s)",
    });
    const repeatedRequirement = { type: "feature", name: "subagents" };
    expect(
      validateWire("ProblemData", {
        type: "capability_not_negotiated",
        requiredCapabilities: [repeatedRequirement, repeatedRequirement],
      }),
    ).toContainEqual({
      path: "ProblemData.requiredCapabilities",
      detail: "expected no repeated items",
    });

    expect(validateWire("ProblemData", { type: "run_lost" })).toEqual([]);
    expect(
      validateWire("ProblemData", { type: "mcp_dial_failed", detail: "connection failed" }),
    ).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "plugin:acme/model_timeout",
        retryAfterSeconds: 2,
      }),
    ).toEqual([]);
    expect(validateWire("ProblemData", { type: "model_timeout" })).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(validateWire("ProblemData", { type: "plugin:Acme/model_timeout" })).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "run_lost",
        activeRun: { runId: "run_1", status: "running" },
      }),
    ).toContainEqual({
      path: "ProblemData",
      detail: "matches no permitted variant",
    });
    expect(
      validateWire("ProblemData", {
        type: "idempotency_in_progress",
        retryAfterSeconds: 0,
      }),
    ).toContainEqual({
      path: "ProblemData.retryAfterSeconds",
      detail: "expected at least 1",
    });
  });
});
