import { describe, expect, it } from "vitest";
import type { TaskReadoutTask } from "./ports/taskReadoutPort";
import { taskProgressPercent, taskReadout } from "./taskReadout";

const task = (id: string, patch: Partial<TaskReadoutTask>): TaskReadoutTask => ({
  id,
  label: id,
  progress: null,
  message: null,
  status: "running",
  startedAt: 0,
  ...patch,
});

describe("taskReadout", () => {
  it("returns null when no task exists", () => {
    expect(taskReadout(new Map())).toBeNull();
  });

  it("sorts by start time and highlights the oldest running task", () => {
    const done = task("done", { status: "succeeded", startedAt: 1 });
    const firstRunning = task("first", { startedAt: 2 });
    const secondRunning = task("second", { startedAt: 3 });

    const readout = taskReadout(
      new Map([
        [secondRunning.id, secondRunning],
        [done.id, done],
        [firstRunning.id, firstRunning],
      ]),
    );

    expect(readout).toMatchObject({
      tasks: [done, firstRunning, secondRunning],
      runningCount: 2,
      head: firstRunning,
      label: "first +1",
      title: "2 running task(s)",
    });
  });

  it("uses the latest settled task when nothing is running", () => {
    const first = task("first", { status: "succeeded", startedAt: 1 });
    const latest = task("latest", { status: "failed", startedAt: 2 });

    expect(
      taskReadout(
        new Map([
          [first.id, first],
          [latest.id, latest],
        ]),
      ),
    ).toMatchObject({
      head: latest,
      label: "latest",
      title: "Recent tasks",
    });
  });
});

describe("taskProgressPercent", () => {
  it("clamps determinate progress and hides failed or indeterminate progress", () => {
    expect(taskProgressPercent(task("half", { progress: 0.456 }))).toBe(46);
    expect(taskProgressPercent(task("over", { progress: 3 }))).toBe(100);
    expect(taskProgressPercent(task("under", { progress: -1 }))).toBe(0);
    expect(taskProgressPercent(task("failed", { status: "failed", progress: 0.8 }))).toBeNull();
    expect(taskProgressPercent(task("none", { progress: null }))).toBeNull();
  });
});
