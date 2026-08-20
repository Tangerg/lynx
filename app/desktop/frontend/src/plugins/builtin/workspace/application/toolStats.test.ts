import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { describe, expect, it } from "vitest";
import { toolStats, toolTimeShare } from "./toolStats";

let seq = 0;
function call(patch: Partial<ToolCall> & Pick<ToolCall, "name" | "status">): ToolCall {
  seq += 1;
  return {
    id: `call_${seq}`,
    runId: "run_1",
    fn: patch.name,
    args: "",
    ...patch,
  };
}

function calls(...list: ToolCall[]): Record<string, ToolCall> {
  return Object.fromEntries(list.map((entry) => [entry.id, entry]));
}

describe("where a session's tool time went", () => {
  it("orders by time spent, not by how often a tool was called", () => {
    const summary = toolStats(
      calls(
        call({ name: "grep", status: "ok", durationMillis: 5 }),
        call({ name: "grep", status: "ok", durationMillis: 5 }),
        call({ name: "grep", status: "ok", durationMillis: 5 }),
        call({ name: "shell", status: "ok", durationMillis: 4000 }),
      ),
    );

    expect(summary.rows.map((row) => row.name)).toEqual(["shell", "grep"]);
    expect(summary.rows[0]).toMatchObject({ calls: 1, totalMs: 4000, slowestMs: 4000 });
  });

  // A call in flight has no outcome and no duration. Counting it would make the
  // totals move backwards the moment it settles.
  it("counts only calls that have finished", () => {
    const summary = toolStats(
      calls(
        call({ name: "shell", status: "running" }),
        call({ name: "shell", status: "requires-action" }),
        call({ name: "shell", status: "ok", durationMillis: 10 }),
      ),
    );

    expect(summary.calls).toBe(1);
  });

  // A denial is a person saying no. Folding it into failures would make an
  // approval policy read as a broken tool.
  it("keeps a refusal apart from a failure", () => {
    const summary = toolStats(
      calls(
        call({ name: "apply_patch", status: "denied" }),
        call({ name: "apply_patch", status: "err" }),
        call({ name: "apply_patch", status: "ok", durationMillis: 3 }),
      ),
    );

    expect(summary.rows[0]).toMatchObject({ calls: 3, failed: 1, denied: 1 });
  });

  it("does not let an untimed call count as instant", () => {
    const summary = toolStats(
      calls(
        call({ name: "read", status: "ok" }),
        call({ name: "read", status: "ok", durationMillis: 8 }),
      ),
    );

    expect(summary.rows[0]).toMatchObject({ calls: 2, timed: 1, totalMs: 8 });
  });

  it("draws no bar when nothing was timed", () => {
    const summary = toolStats(calls(call({ name: "read", status: "ok" })));
    expect(toolTimeShare(summary.rows[0]!, summary)).toBe(0);
  });
});
