// Regression: stdout that streams via item.delta{toolOutput} during a HITL
// *resume* run must land on the toolCall's `result`. Captured from the real
// runtime: a commandExecution interrupts for approval (runs.start ends with
// run.finished{interrupt}, toolCall still inProgress, no output yet), then the
// resume run RE-EMITS the same toolCall id and streams its stdout before
// settling. The fold must preserve the resume-streamed output onto the
// pre-existing toolCalls entry — not reset it when item.started re-fires.
import { beforeEach, describe, expect, it } from "vitest";
import type { Item, RunOutcome, StreamEvent } from "@/rpc";
import type { AgentViewState } from "./viewState";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_X",
    status: "running",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;
const runStarted = (id: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId: "ses_1" } as never,
});
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "run.finished", outcome });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

const TOOL = "item_run_X_1";

describe("reducer — HITL resume preserves toolOutput on result", () => {
  it("stdout streamed during the resume run lands on the re-emitted toolCall", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;

    // runs.start: command interrupts for approval before executing.
    s = reduce(s, runStarted("run_X"));
    s = reduce(
      s,
      started(
        item({ id: TOOL, type: "toolCall", tool: { name: "bash", arguments: { command: "pwd" } } }),
      ),
    );
    s = reduce(
      s,
      runFinished({
        type: "interrupt",
        interrupts: [
          {
            itemId: TOOL,
            type: "approval",
            payload: { tool: { name: "bash", arguments: { command: "pwd" } } },
          },
        ],
      } as never),
    );
    expect(s.toolCalls[TOOL]?.result).toBeUndefined();

    // runs.resume: re-emits the same toolCall id, then streams stdout + settles.
    s = reduce(s, runStarted("run_X_resume"));
    s = reduce(
      s,
      started(
        item({ id: TOOL, type: "toolCall", tool: { name: "bash", arguments: { command: "pwd" } } }),
      ),
    );
    s = reduce(s, delta(TOOL, { type: "toolArguments", argumentsTextDelta: '{"command": "pwd"}' }));
    s = reduce(s, delta(TOOL, { type: "toolOutput", text: "/Users/tangerg\n" }));
    s = reduce(
      s,
      completed(
        item({
          id: TOOL,
          status: "completed",
          type: "toolCall",
          tool: { name: "bash", arguments: { command: "pwd" }, result: { exitCode: 0 } },
        }),
      ),
    );

    expect(s.toolCalls[TOOL]?.result).toBe("/Users/tangerg\n");
    expect(s.toolCalls[TOOL]?.status).toBe("ok");
    expect(s.toolCalls[TOOL]?.exitCode).toBe(0);
  });
});
