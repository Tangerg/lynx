import { describe, expect, it } from "vitest";
import {
  activePlanStep,
  planProgress,
  planSteps,
  planStepsFromArguments,
  planStepsFromToolArgs,
  SessionPlan,
} from "./sessionPlan";

describe("planSteps", () => {
  it("reads the Adapter-owned checklist projection", () => {
    expect(
      planSteps({
        revision: 1,
        steps: [
          { id: "1", text: "Read the code", status: "done" },
          { id: "2", text: "Write the fix", status: "active" },
          { id: "3", text: "Run tests", status: "pending" },
        ],
      }),
    ).toEqual([
      { id: "1", text: "Read the code", status: "done" },
      { id: "2", text: "Write the fix", status: "active" },
      { id: "3", text: "Run tests", status: "pending" },
    ]);
  });

  it("has no steps without a snapshot", () => {
    expect(planSteps(undefined)).toEqual([]);
  });
});

describe("SessionPlan", () => {
  const snapshot = {
    revision: 7,
    steps: [{ id: "1", text: "Inspect", status: "active" as const }],
  };

  it("uses exact projection generation, Session and whole-replacement revision as identity", () => {
    const current = SessionPlan.fromSnapshot("ses-a", 2, snapshot);
    expect(SessionPlan.fromSnapshot("ses-a", 2, snapshot).identity).toBe(current.identity);
    expect(SessionPlan.fromSnapshot("ses-a", 2, { ...snapshot, revision: 8 }).identity).not.toBe(
      current.identity,
    );
    expect(SessionPlan.fromSnapshot("ses-b", 2, snapshot).identity).not.toBe(current.identity);
    expect(SessionPlan.fromSnapshot("ses-a", 3, snapshot).identity).not.toBe(current.identity);
  });

  it("owns active-step and completion behavior", () => {
    const plan = SessionPlan.fromSnapshot("ses-a", 2, {
      revision: 8,
      steps: [
        { id: "1", text: "Inspect", status: "done" },
        { id: "2", text: "Fix", status: "active" },
      ],
    });
    expect(plan.activeStep()?.text).toBe("Fix");
    expect(plan.progress()).toEqual({ done: 1, total: 2 });
  });
});

describe("planStepsFromArguments", () => {
  it("reads a set_plan call's own steps, indexing them for keys", () => {
    expect(
      planStepsFromArguments({
        steps: [
          { description: "Read the code", status: "completed" },
          { description: "Write the fix", status: "in_progress" },
        ],
      }),
    ).toEqual([
      { id: "0", text: "Read the code", status: "done" },
      { id: "1", text: "Write the fix", status: "active" },
    ]);
  });

  it("treats an unknown status as not started rather than dropping the step", () => {
    expect(
      planStepsFromArguments({ steps: [{ description: "Ship", status: "elsewhere" }] }),
    ).toEqual([{ id: "0", text: "Ship", status: "pending" }]);
  });

  it("skips entries that carry no description, and anything that is not a plan", () => {
    expect(planStepsFromArguments({ steps: [{ status: "pending" }, { description: "" }] })).toEqual(
      [],
    );
    expect(planStepsFromArguments({ steps: "nope" })).toEqual([]);
    expect(planStepsFromArguments(null)).toEqual([]);
  });
});

describe("planStepsFromToolArgs", () => {
  it("parses the accumulated argument text", () => {
    expect(
      planStepsFromToolArgs(
        JSON.stringify({ steps: [{ description: "Ship", status: "pending" }] }),
      ),
    ).toEqual([{ id: "0", text: "Ship", status: "pending" }]);
  });

  it("reads a half-streamed call as no plan, not as a broken one", () => {
    expect(planStepsFromToolArgs('{"steps":[{"desc')).toEqual([]);
    expect(planStepsFromToolArgs("")).toEqual([]);
  });
});

describe("what a row reports", () => {
  const steps = planStepsFromArguments({
    steps: [
      { description: "Read the code", status: "completed" },
      { description: "Write the fix", status: "in_progress" },
      { description: "Run tests", status: "pending" },
    ],
  });

  it("watches the step in flight", () => {
    expect(activePlanStep(steps)?.text).toBe("Write the fix");
  });

  it("watches the step in flight even when an untouched one comes first", () => {
    const outOfOrder = planStepsFromArguments({
      steps: [
        { description: "Write the docs", status: "pending" },
        { description: "Write the fix", status: "in_progress" },
      ],
    });
    expect(activePlanStep(outOfOrder)?.text).toBe("Write the fix");
  });

  it("falls back to the next step not started", () => {
    const notStarted = planStepsFromArguments({
      steps: [
        { description: "Read the code", status: "completed" },
        { description: "Write the fix", status: "pending" },
      ],
    });
    expect(activePlanStep(notStarted)?.text).toBe("Write the fix");
  });

  it("has nothing to watch once every step is done", () => {
    const done = planStepsFromArguments({
      steps: [
        { description: "Read the code", status: "completed" },
        { description: "Ship it", status: "completed" },
      ],
    });
    expect(activePlanStep(done)).toBeUndefined();
  });

  it("has nothing to watch in an empty plan", () => {
    expect(activePlanStep([])).toBeUndefined();
  });

  it("counts how far the plan has got", () => {
    expect(planProgress(steps)).toEqual({ done: 1, total: 3 });
    expect(planProgress([])).toEqual({ done: 0, total: 0 });
  });
});
