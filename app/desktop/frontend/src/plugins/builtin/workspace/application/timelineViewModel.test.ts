import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { AgentRunView, TimelineEntry } from "@/plugins/builtin/agent/public/viewState";
import type { AgentRunTreeNode } from "@/plugins/builtin/agent/public/run";
import {
  timelineGroupKey,
  timelineRunStatusView,
  timelineSubtext,
  timelineTimeOfDay,
  timelineViewModel,
} from "./timelineViewModel";

const entry = (id: string, runId: string | null): TimelineEntry => ({
  id,
  runId,
  kind: "run-start",
  ts: 0,
});

function run(id: string, overrides: Partial<AgentRunView> = {}): AgentRunView {
  return {
    id,
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: id,
    spawnedByItemId: null,
    status: "running",
    activeSegmentId: `segment-${id}`,
    outcome: null,
    metrics: {
      steps: 2,
      activeDurationMillis: 10,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: { step: 3, activity: "Inspecting" },
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: null,
    ...overrides,
  };
}

function node(value: AgentRunView, children: AgentRunTreeNode[] = []): AgentRunTreeNode {
  return { run: value, children };
}

describe("timelineViewModel", () => {
  it("groups all events by lineage instead of adjacent arrival order", () => {
    const root = run("root");
    const child = run("child", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "task-item",
    });
    const view = timelineViewModel(
      [
        entry("session", null),
        entry("root-1", root.id),
        entry("child-1", child.id),
        entry("root-2", root.id),
      ],
      [node(root, [node(child)])],
    );

    expect(view).toMatchObject({ eventCount: 4, runCount: 2 });
    expect(
      view.groups.map(({ runId, depth, items }) => [runId, depth, items.map((x) => x.id)]),
    ).toEqual([
      [null, 0, ["session"]],
      ["root", 0, ["root-1", "root-2"]],
      ["child", 1, ["child-1"]],
    ]);
  });

  it("retains Runs without events and events whose Run is unknown", () => {
    const root = run("root");
    const view = timelineViewModel([entry("unknown-event", "unknown")], [node(root)]);
    expect(view.groups.map((group) => [group.runId, group.run?.id ?? null])).toEqual([
      ["root", "root"],
      ["unknown", null],
    ]);
  });
});

describe("timeline Run status", () => {
  it("projects exact progress and cancelability", () => {
    expect(timelineRunStatusView(run("running"))).toMatchObject({
      state: "running",
      labelKey: "agent.runTree.status.running",
      tone: "accent",
      detail: "Inspecting",
      stepCount: 3,
      cancelable: true,
    });
  });

  it("projects terminal error without offering cancel", () => {
    expect(
      timelineRunStatusView(
        run("failed", {
          status: "finished",
          activeSegmentId: null,
          progress: null,
          outcome: { type: "failed", error: { message: "Provider failed" } },
          finishedAt: "2026-01-01T00:00:01.000Z",
        }),
      ),
    ).toMatchObject({
      state: "error",
      tone: "negative",
      detail: "Provider failed",
      stepCount: 2,
      cancelable: false,
    });
  });
});

describe("timeline view helpers", () => {
  it("builds stable keys and header subtext", () => {
    expect(timelineGroupKey({ runId: null, run: null, depth: 0, items: [] })).toBe("session");
    expect(timelineGroupKey({ runId: "run-a", run: run("run-a"), depth: 0, items: [] })).toBe(
      "run-a",
    );
    expect(timelineSubtext(t, { eventCount: 0, runCount: 0 })).toBe("0 events · 0 run(s)");
    expect(timelineSubtext(t, { eventCount: 3, runCount: 2 })).toBe("3 events · 2 run(s)");
  });

  it("formats timestamps as local time of day", () => {
    const date = new Date(2024, 0, 2, 3, 4, 5);
    expect(timelineTimeOfDay(date.getTime())).toBe("03:04:05");
  });
});
