// Activity event semantics — ACTIVITY_SNAPSHOT (merge / replace /
// scoping) and ACTIVITY_DELTA (JSON Patch onto prior content, with
// empty-object fallback and broken-patch isolation).

import type {BaseEvent} from "@ag-ui/core";
import type {AgentViewState} from "@/protocol/agui/viewState";
import {  EventType } from "@ag-ui/core";
import { beforeEach, describe, expect, it } from "vitest";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "@/protocol/agui/reducer";
import {  INITIAL_VIEW_STATE  } from "@/protocol/agui/viewState";

const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

function withAssistantMessage(id = "m1"): AgentViewState {
  return reduce(
    INITIAL_VIEW_STATE,
    ev({
      type: EventType.TEXT_MESSAGE_START,
      messageId: id,
      role: "assistant",
    }),
  );
}

// ---------------------------------------------------------------------------
// ACTIVITY_SNAPSHOT
// ---------------------------------------------------------------------------

describe("core-reducer — ACTIVITY_SNAPSHOT", () => {
  it("writes content onto message.activities under the activityType key", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { query: "react query", hits: 12 },
      }),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.websearch).toEqual({ query: "react query", hits: 12 });
  });

  it("merges with prior content by default (replace=false)", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { query: "q1", hits: 5 },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { hits: 10, latencyMs: 200 },
      }),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    // hits gets overwritten (10), query preserved (q1), latencyMs added.
    expect(m.activities?.websearch).toEqual({ query: "q1", hits: 10, latencyMs: 200 });
  });

  it("replaces wholesale when replace=true", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { query: "q1", hits: 5 },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { totalMs: 400 },
        replace: true,
      } as BaseEvent),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.websearch).toEqual({ totalMs: 400 });
  });

  it("scopes by (messageId, activityType) — different keys coexist", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "websearch",
        content: { hits: 1 },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "exec",
        content: { exitCode: 0 },
      }),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities).toEqual({
      websearch: { hits: 1 },
      exec: { exitCode: 0 },
    });
  });
});

// ---------------------------------------------------------------------------
// ACTIVITY_DELTA
// ---------------------------------------------------------------------------

describe("core-reducer — ACTIVITY_DELTA", () => {
  it("applies a JSON Patch to the existing activity content", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "exec",
        content: { stdout: "", stderr: "" },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_DELTA,
        messageId: "m1",
        activityType: "exec",
        patch: [
          { op: "replace", path: "/stdout", value: "line one\n" },
          { op: "add", path: "/exitCode", value: 0 },
        ],
      } as BaseEvent),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({
      stdout: "line one\n",
      stderr: "",
      exitCode: 0,
    });
  });

  it("starts from {} when no prior content exists for the activity", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_DELTA,
        messageId: "m1",
        activityType: "exec",
        patch: [{ op: "add", path: "/started", value: true }],
      } as BaseEvent),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({ started: true });
  });

  it("with a broken patch leaves prior content unchanged", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_SNAPSHOT,
        messageId: "m1",
        activityType: "exec",
        content: { stdout: "kept" },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.ACTIVITY_DELTA,
        messageId: "m1",
        activityType: "exec",
        patch: [{ op: "remove", path: "/does/not/exist" }],
      } as BaseEvent),
    );
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({ stdout: "kept" });
  });
});
