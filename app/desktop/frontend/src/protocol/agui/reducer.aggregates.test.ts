// Reducer — accumulator-shape tests. These cover the *view-level*
// data structures the reducer maintains alongside the message stream:
// the audit `timeline`, the agent-owned shared state (snapshot +
// JSON-patch delta), and bulk `MESSAGES_SNAPSHOT` hydration.

import type {BaseEvent} from "@ag-ui/core";
import type {AgentViewState} from "./viewState";
import {  EventType } from "@ag-ui/core";
import { beforeEach, describe, expect, it } from "vitest";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { CUSTOM } from "./customEvents";
import { reduce } from "./reducer";
import {  INITIAL_VIEW_STATE } from "./viewState";

const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — timeline accumulator", () => {
  it("records run-start / tool-start+end / run-end entries in order", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.RUN_STARTED, threadId: "t", runId: "r1" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_START,
        toolCallId: "tc1",
        toolCallName: "bash",
        parentMessageId: "m1",
      }),
    );
    s = reduce(s, ev({ type: EventType.TOOL_CALL_END, toolCallId: "tc1" }));
    s = reduce(s, ev({ type: EventType.RUN_FINISHED, threadId: "t", runId: "r1" }));

    const kinds = s.timeline.map((t) => t.kind);
    expect(kinds).toEqual(["run-start", "tool-start", "tool-end", "run-end"]);
    expect(s.timeline.every((t) => t.runId === "r1")).toBe(true);
    expect(s.timeline.find((t) => t.kind === "tool-end")?.status).toBe("ok");
    expect(s.timeline.find((t) => t.kind === "tool-start")?.summary).toBe("bash");
  });

  it("records approval-request + approval-result when handler loaded", async () => {
    const { approvalHandler: spec } = await import("@/plugins/builtin/agui-handlers");
    await loadPlugin(spec);

    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.RUN_STARTED, threadId: "t", runId: "r1" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(
      s,
      ev({
        type: EventType.CUSTOM,
        name: CUSTOM.APPROVAL,
        value: {
          requestId: "req-1",
          parentMessageId: "m1",
          text: "Run migration?",
          command: "psql -f migrate.sql",
          reason: "deploy",
        },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.CUSTOM,
        name: CUSTOM.APPROVAL_RESULT,
        value: { requestId: "req-1", decision: "approved" },
      }),
    );

    const approval = s.timeline.filter((t) => t.kind.startsWith("approval"));
    expect(approval.map((t) => t.kind)).toEqual(["approval-request", "approval-result"]);
    expect(approval[0].refId).toBe("req-1");
    expect(approval[1].status).toBe("approved");
  });
});

describe("reducer — state snapshot / delta", () => {
  it("sTATE_SNAPSHOT replaces shared wholesale", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.STATE_SNAPSHOT,
        snapshot: { plan: ["a", "b"], counter: 1 },
      }),
    );
    expect(s.shared).toEqual({ plan: ["a", "b"], counter: 1 });
  });

  it("sTATE_DELTA applies a JSON Patch to shared", () => {
    let s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.STATE_SNAPSHOT,
        snapshot: { counter: 0, list: ["a"] },
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.STATE_DELTA,
        delta: [
          { op: "replace", path: "/counter", value: 5 },
          { op: "add", path: "/list/-", value: "b" },
        ],
      }),
    );
    expect(s.shared).toEqual({ counter: 5, list: ["a", "b"] });
  });

  it("sTATE_DELTA with a broken patch leaves shared unchanged", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.STATE_SNAPSHOT,
        snapshot: { x: 1 },
      }),
    );
    const next = reduce(
      s,
      ev({
        type: EventType.STATE_DELTA,
        delta: [{ op: "remove", path: "/does/not/exist" }],
      }),
    );
    expect(next.shared).toEqual({ x: 1 });
  });
});

describe("reducer — messages snapshot", () => {
  it("hydrates messages and toolCalls from a snapshot", () => {
    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.MESSAGES_SNAPSHOT,
        messages: [
          { id: "u1", role: "user", content: "hi" },
          {
            id: "a1",
            role: "assistant",
            content: "ok",
            toolCalls: [
              {
                id: "t1",
                type: "function",
                function: { name: "read_file", arguments: '{"path":"x"}' },
              },
            ],
          },
          { id: "tr1", role: "tool", toolCallId: "t1", content: "file contents" },
        ],
      }),
    );

    expect(next.messages.map((m) => m.role)).toEqual(["user", "assistant"]);
    expect(next.messages[1].blocks).toEqual([
      { kind: "text", text: "ok", status: "complete" },
      { kind: "tool", toolCallId: "t1" },
    ]);
    expect(next.toolCalls.t1.fn).toBe("read_file");
    expect(next.toolCalls.t1.result).toBe("file contents");
  });
});
