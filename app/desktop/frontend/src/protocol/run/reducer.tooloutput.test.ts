// Regression: commandExecution output must survive every path, not just live
// streaming. The authoritative merged stdout lands on the completed item's
// `output` (durable); the toolOutput ItemDelta is only a live preview. So
// history replay (completed-only, no deltas), reconnect, and non-streaming
// runtimes must all still render output. See docs/TOOL_OUTPUT.md + API.md §5.2.
import { beforeEach, describe, expect, it } from "vitest";
import type { Item, StreamEvent } from "@/rpc";
import type { AgentViewState } from "./viewState";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_1",
    status: "inProgress",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

const cmd = (extra: Record<string, unknown>) => ({
  kind: "commandExecution",
  command: ["pwd"],
  ...extra,
});

describe("reducer — commandExecution output durability", () => {
  it("history replay (completed-only, no deltas) renders output from tool.output", () => {
    // items.list hydration replays ONLY completed items — no item.delta ever.
    const s = reduce(
      INITIAL_VIEW_STATE,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
    expect(s.toolCalls["t1"]?.exitCode).toBe(0);
  });

  it("completed `output` is authoritative — overrides an incomplete delta preview", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "t1", type: "toolCall", tool: cmd({}) })));
    // Only a partial preview streamed (frames dropped / slow stream).
    s = reduce(s, delta("t1", { type: "toolOutput", text: "/Users" }));
    expect(s.toolCalls["t1"]?.result).toBe("/Users"); // live preview
    // Completed carries the full authoritative output → it wins.
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "/Users/tangerg\n", exitCode: 0 }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
  });

  it("while inProgress the toolOutput delta is the live preview (no settled fields yet)", () => {
    // The started shell carries no output (lifecycle); the delta stream stands
    // in as preview until item.completed reconciles to the authoritative output.
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "t1", type: "toolCall", tool: cmd({}) })));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "/Users/tan" }));
    s = reduce(s, delta("t1", { type: "toolOutput", text: "gerg\n" }));
    expect(s.toolCalls["t1"]?.status).toBe("running");
    expect(s.toolCalls["t1"]?.result).toBe("/Users/tangerg\n");
  });

  it("outputTruncated rides through to the view when the runtime caps output", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      completed(
        item({
          id: "t1",
          status: "completed",
          type: "toolCall",
          tool: cmd({ output: "first 64KB…\n", exitCode: 0, outputTruncated: true }),
        }),
      ),
    );
    expect(s.toolCalls["t1"]?.result).toBe("first 64KB…\n");
    expect(s.toolCalls["t1"]?.outputTruncated).toBe(true);
  });
});
