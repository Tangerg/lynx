// Subagent run isolation. A delegated sub-agent shares the parent's event
// stream (same SSE connection), so its run.* events arrive interleaved with the
// root run's. The fold must keep them from clobbering the root run readout: a
// child run is a timeline entry, never a reset of running / runId / step /
// usage. The discriminators are run.spawnedByItemId on run.started and the wire
// envelope runId (reduce's third arg) on run.progress / run.finished.

import { beforeEach, describe, expect, it } from "vitest";
import type { RunOutcome, StreamEvent } from "@/rpc";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";

const runStarted = (id: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId: "ses_1" } as never,
});
const subagentStarted = (id: string, spawnedByItemId: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId: "ses_1", spawnedByItemId } as never,
});
const progress = (p: Record<string, unknown>): StreamEvent =>
  ({ type: "run.progress", progress: p }) as StreamEvent;
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "run.finished", outcome });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("reducer — subagent run isolation", () => {
  it("a subagent run.started records a timeline entry but does not reset the root run", () => {
    let s: AgentViewState = reduce(INITIAL_VIEW_STATE, runStarted("run_1"));
    s = reduce(s, progress({ step: 3, activity: "root work" }));

    s = reduce(s, subagentStarted("sub_1", "tool_x"));

    // Root readout untouched — still the root run, still running, still step 3.
    expect(s.run).toMatchObject({ running: true, runId: "run_1", step: 3, activity: "root work" });
    // The child boundary is only a timeline marker.
    expect(s.timeline.findLast((e) => e.kind === "run-start")).toMatchObject({
      runId: "sub_1",
      summary: "subagent",
    });
  });

  it("a subagent run.progress (mismatched envelope runId) does not overwrite the root readout", () => {
    let s: AgentViewState = reduce(INITIAL_VIEW_STATE, runStarted("run_1"));
    s = reduce(s, progress({ step: 5, activity: "root work" }));

    // Child progress carries the child's runId as the envelope discriminator.
    s = reduce(s, progress({ step: 99, activity: "subagent work" }), "sub_1");

    expect(s.run).toMatchObject({ step: 5, activity: "root work" });
  });

  it("a subagent run.finished (mismatched envelope runId) leaves the root run running", () => {
    let s: AgentViewState = reduce(INITIAL_VIEW_STATE, runStarted("run_1"));

    s = reduce(s, runFinished({ type: "completed", result: { steps: 1 } }), "sub_1");

    // The root run owns running/interrupt state — only its own finished flips it.
    expect(s.run.running).toBe(true);
    expect(s.timeline.findLast((e) => e.kind === "run-end")).toMatchObject({
      runId: "sub_1",
      summary: "subagent",
    });
  });
});
