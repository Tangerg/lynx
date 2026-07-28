import { describe, expect, it } from "vitest";

import { validateWire } from "./wire.validate.generated";

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
  cwd: "/Users/dev/project",
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
  revision: 3,
};

const artifactSession = {
  id: "ses_01",
  title: "Refactor the runtime protocol",
  model: "claude-opus-4-8",
  cwd: "/Users/dev/project",
  createdAt: "2026-07-07T10:00:00Z",
  updatedAt: "2026-07-07T10:05:00Z",
};

const finishedRun = {
  id: "run_01",
  sessionId: "ses_01",
  status: "finished",
  createdAt: "2026-07-07T10:00:00Z",
  finishedAt: "2026-07-07T10:00:20Z",
  outcome: { type: "completed", result: { steps: 2, durationMs: 20_000 } },
};

describe("the generated wire checks", () => {
  it("accepts a well-formed frame", () => {
    expect(validateWire("Session", session)).toEqual([]);
  });

  it("names every missing required field", () => {
    const { id: _id, revision: _revision, ...partial } = session;
    expect(validateWire("Session", partial)).toEqual([
      { path: "Session.id", detail: "is required" },
      { path: "Session.revision", detail: "is required" },
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

  // The bundle carries no `additionalProperties: false`, because the runtime's
  // decoder ignores what it does not know. Refusing an unknown field here would
  // reject frames the runtime accepts — and break every forward-compatible client.
  it("ignores a property the contract does not mention", () => {
    expect(validateWire("Session", { ...session, inventedByANewerServer: true })).toEqual([]);
  });

  it("rejects an empty string where the contract states a minimum length", () => {
    expect(validateWire("GetSessionRequest", { sessionId: "ses_01" })).toEqual([]);
    expect(validateWire("GetSessionRequest", { sessionId: "" })).toEqual([
      { path: "GetSessionRequest.sessionId", detail: "expected at least 1 character(s)" },
    ]);
  });

  it("rejects a revision below the minimum", () => {
    expect(
      validateWire("UpdateSessionRequest", { sessionId: "ses_01", expectedRevision: 0 }),
    ).toEqual([{ path: "UpdateSessionRequest.expectedRevision", detail: "expected at least 1" }]);
  });

  // The constraint belongs to this request, not to every carrier of the shared
  // shape, so the schema states it in an allOf branch — a third code path, and the
  // one that reads `minLength` with no type keyword beside it.
  it("states a constraint on a field of a shared shape", () => {
    const artifact = {
      version: 7,
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

  it("refuses a discriminator no variant claims", () => {
    const details = validateWire("ContentBlock", { type: "video", data: "AAAA" }).map(
      (v) => v.detail,
    );
    expect(details).toContain("matches no permitted variant");
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
  });
});
