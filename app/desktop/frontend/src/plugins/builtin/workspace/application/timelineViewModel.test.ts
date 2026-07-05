import { describe, expect, it } from "vitest";
import type { TimelineEntry } from "@/plugins/builtin/agent/public/viewState";
import {
  groupTimelineByRun,
  timelineGroupKey,
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

describe("groupTimelineByRun", () => {
  it("groups consecutive entries by run id and keeps pre-run entries separate", () => {
    const groups = groupTimelineByRun([
      entry("pre", null),
      entry("a1", "run-a"),
      entry("a2", "run-a"),
      entry("pre-2", null),
      entry("a3", "run-a"),
    ]);

    expect(groups).toEqual([
      { runId: null, items: [entry("pre", null)] },
      { runId: "run-a", items: [entry("a1", "run-a"), entry("a2", "run-a")] },
      { runId: null, items: [entry("pre-2", null)] },
      { runId: "run-a", items: [entry("a3", "run-a")] },
    ]);
  });
});

describe("timelineViewModel", () => {
  it("projects event count, run group count, and groups", () => {
    expect(timelineViewModel([entry("a", "run-a"), entry("b", "run-b")])).toMatchObject({
      eventCount: 2,
      runCount: 2,
      groups: [
        { runId: "run-a", items: [entry("a", "run-a")] },
        { runId: "run-b", items: [entry("b", "run-b")] },
      ],
    });
  });
});

describe("timeline view helpers", () => {
  it("builds stable group keys and header subtext", () => {
    expect(timelineGroupKey({ runId: null, items: [] }, 0)).toBe("pre:0");
    expect(timelineGroupKey({ runId: "run-a", items: [] }, 2)).toBe("run-a:2");
    expect(timelineSubtext({ eventCount: 0, runCount: 0 })).toBe("0 events · 0 runs");
    expect(timelineSubtext({ eventCount: 3, runCount: 1 })).toBe("3 events · 1 run");
  });

  it("formats timestamps as local time of day", () => {
    const date = new Date(2024, 0, 2, 3, 4, 5);
    expect(timelineTimeOfDay(date.getTime())).toBe("03:04:05");
  });
});
