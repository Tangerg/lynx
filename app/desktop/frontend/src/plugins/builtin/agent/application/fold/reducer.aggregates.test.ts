// Reducer — accumulator-shape tests. These cover the *view-level* data
// structures the reducer maintains alongside the message stream: the audit
// `timeline`, the Runtime-owned Plan, and durable
// history hydration via item.completed.

import { beforeEach, describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { noMetrics } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

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
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

describe("reducer — timeline accumulator", () => {
  it("records run-start / tool-start+end / run-end entries in order", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
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
    s = reduce(s, { type: "segment.finished", outcome: { type: "completed" }, metrics: noMetrics });

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
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
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
      metrics: noMetrics,
      outcome: {
        type: "interrupt",
        interrupts: [
          {
            itemId: "tc1" as never,
            runId: "r1" as never,
            type: "approval",
            payload: {
              tool: { name: "shell", arguments: { command: "psql" } },
              rememberable: true,
            },
          },
        ],
      },
    });
    const approval = s.timeline.filter((t) => t.kind.startsWith("approval"));
    expect(approval.map((t) => t.kind)).toEqual(["approval-request"]);
    expect(approval[0]!.refId).toBe("tc1");
  });
});

describe("reducer — plan", () => {
  const plan = (revision: number, description: string): StreamEvent => ({
    type: "plan.updated",
    plan: {
      revision,
      steps: [{ id: "1", text: description, status: "pending" }],
    },
  });

  it("a plan update replaces the Plan wholesale", () => {
    const s = reduce(EMPTY_AGENT_SESSION_VIEW, plan(1, "first"));
    expect(s.plan).toMatchObject({ revision: 1, steps: [{ text: "first" }] });
  });

  // The list is replaced whole, so contents cannot say which snapshot is later — an
  // older one arriving late would look exactly like progress being undone.
  it("an older revision does not overwrite a newer one", () => {
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, plan(4, "current"));
    s = reduce(s, plan(2, "stale"));
    expect(s.plan).toMatchObject({ revision: 4, steps: [{ text: "current" }] });
  });

  it("a duplicate revision is a no-op even if a drifted replay arrives", () => {
    const current = reduce(EMPTY_AGENT_SESSION_VIEW, plan(4, "current"));
    const duplicate = reduce(current, plan(4, "drifted replay"));

    expect(duplicate).toBe(current);
    expect(duplicate.plan).toMatchObject({
      revision: 4,
      steps: [{ text: "current" }],
    });
  });
});

describe("reducer — durable history hydration", () => {
  it("item.completed without a prior item.started upserts the block (items.list replay)", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
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
