// Reducer — accumulator-shape tests. These cover the *view-level* data
// structures the reducer maintains alongside the message stream: the audit
// `timeline`, the agent-owned shared state (state.snapshot), and durable
// history hydration via item.completed.

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, StreamEvent } from "@/rpc";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "r1",
    status: "running",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("reducer — timeline accumulator", () => {
  it("records run-start / tool-start+end / run-end entries in order", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, { type: "segment.started", run: { id: "r1", sessionId: "s" } as never });
    s = reduce(
      s,
      started(
        item({
          id: "tc1",
          type: "toolCall",
          tool: { name: "shell", arguments: { command: "ls" } },
        }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "tc1",
          type: "toolCall",
          status: "completed",
          tool: { name: "shell", arguments: { command: "ls" } },
        }),
      ),
    );
    s = reduce(s, { type: "segment.finished", outcome: { type: "completed", result: {} } });

    expect(s.timeline.map((t) => t.kind)).toEqual([
      "run-start",
      "tool-start",
      "tool-end",
      "run-end",
    ]);
    expect(s.timeline.every((t) => t.runId === "r1")).toBe(true);
    expect(s.timeline.find((t) => t.kind === "tool-end")?.status).toBe("ok");
    expect(s.timeline.find((t) => t.kind === "tool-start")?.summary).toBe("ls");
  });

  it("records an approval-request when a run finishes with an approval interrupt", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, { type: "segment.started", run: { id: "r1", sessionId: "s" } as never });
    s = reduce(
      s,
      started(
        item({
          id: "tc1",
          type: "toolCall",
          tool: { name: "shell", arguments: { command: "psql" } },
        }),
      ),
    );
    s = reduce(s, {
      type: "segment.finished",
      outcome: {
        type: "interrupt",
        interrupts: [
          {
            itemId: "tc1" as never,
            type: "approval",
            payload: { tool: { name: "shell", arguments: { command: "psql" } } },
          },
        ],
      },
    });
    const approval = s.timeline.filter((t) => t.kind.startsWith("approval"));
    expect(approval.map((t) => t.kind)).toEqual(["approval-request"]);
    expect(approval[0]!.refId).toBe("tc1");
  });
});

describe("reducer — shared state", () => {
  it("state.snapshot replaces shared wholesale", () => {
    const s = reduce(INITIAL_VIEW_STATE, {
      type: "state.snapshot",
      state: { plan: ["a", "b"], counter: 1 },
    });
    expect(s.shared).toEqual({ plan: ["a", "b"], counter: 1 });
  });

});

describe("reducer — durable history hydration", () => {
  it("item.completed without a prior item.started upserts the block (items.list replay)", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      completed(
        item({
          id: "u1",
          type: "userMessage",
          status: "completed",
          content: [{ type: "text", text: "hi" }],
        }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "a1",
          type: "agentMessage",
          status: "completed",
          content: [{ type: "text", text: "hello" }],
        }),
      ),
    );
    expect(s.messages.map((m) => m.role)).toEqual(["user", "assistant"]);
    expect(s.messages[1]!.blocks[0]).toMatchObject({
      kind: "text",
      text: "hello",
      status: "complete",
    });
  });
});
