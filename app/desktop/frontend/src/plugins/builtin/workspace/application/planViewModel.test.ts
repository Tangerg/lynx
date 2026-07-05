import { describe, expect, it } from "vitest";
import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { planSubtext, planViewModel } from "./planViewModel";

const item = (over: Partial<PlanItem>): PlanItem => ({
  id: 1,
  pid: "p1",
  status: "todo",
  text: "Inspect workspace",
  ...over,
});

describe("planViewModel", () => {
  it("counts completed plan items without reordering the plan", () => {
    const first = item({ id: 1, pid: "p1", status: "done" });
    const second = item({ id: 2, pid: "p2", status: "doing" });
    const third = item({ id: 3, pid: "p3", status: "todo" });

    expect(planViewModel([first, second, third])).toEqual({
      items: [first, second, third],
      doneCount: 1,
      totalCount: 3,
      isEmpty: false,
    });
  });

  it("projects an empty plan", () => {
    expect(planViewModel([])).toEqual({
      items: [],
      doneCount: 0,
      totalCount: 0,
      isEmpty: true,
    });
  });
});

describe("planSubtext", () => {
  it("omits header subtext for an empty plan", () => {
    expect(planSubtext({ doneCount: 0, totalCount: 0 })).toBeUndefined();
  });

  it("builds completion subtext", () => {
    expect(planSubtext({ doneCount: 2, totalCount: 3 })).toBe("2 of 3 complete");
  });
});
