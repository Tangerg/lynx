import { describe, expect, it } from "vitest";
import {
  canSaveScheduleDraft,
  initialScheduleDraft,
  scheduleInputFromDraft,
} from "./scheduleDraft";

describe("scheduleDraft", () => {
  it("initializes new schedules from the active cwd", () => {
    expect(initialScheduleDraft(undefined, "/repo")).toEqual({
      title: "",
      instructions: "",
      cron: "0 9 * * 1-5",
      cwd: "/repo",
    });
  });

  it("initializes existing schedules from persisted fields", () => {
    expect(
      initialScheduleDraft(
        {
          id: "sched-1",
          title: "Morning review",
          instructions: "Summarize changes",
          cwd: "/workspace",
          cron: "0 9 * * 1",
          enabled: true,
          createdAt: "2026-01-01T00:00:00Z",
          revision: 1,
        },
        "/ignored",
      ),
    ).toEqual({
      title: "Morning review",
      instructions: "Summarize changes",
      cron: "0 9 * * 1",
      cwd: "/workspace",
    });
  });

  it("does not relocate a default-workspace schedule to the active session", () => {
    expect(
      initialScheduleDraft(
        {
          id: "sched-default",
          title: "Default review",
          instructions: "Review the default workspace",
          cron: "0 9 * * 1",
          enabled: true,
          createdAt: "2026-01-01T00:00:00Z",
          revision: 1,
        },
        "/active-session",
      ).cwd,
    ).toBe("");
  });

  it("builds the schedules create/update input from trimmed form text", () => {
    expect(
      scheduleInputFromDraft({
        title: " Weekly review ",
        instructions: " Review this repo ",
        cron: " 0 9 * * 1 ",
        cwd: " /repo ",
      }),
    ).toEqual({
      title: "Weekly review",
      instructions: "Review this repo",
      cron: "0 9 * * 1",
      cwd: "/repo",
    });
  });

  it("requires instructions and cron while respecting the busy flag", () => {
    const draft = initialScheduleDraft();

    expect(canSaveScheduleDraft({ ...draft, instructions: "run", cron: "0 * * * *" }, false)).toBe(
      true,
    );
    expect(canSaveScheduleDraft({ ...draft, instructions: "", cron: "0 * * * *" }, false)).toBe(
      false,
    );
    expect(canSaveScheduleDraft({ ...draft, instructions: "run", cron: "" }, false)).toBe(false);
    expect(canSaveScheduleDraft({ ...draft, instructions: "run", cron: "0 * * * *" }, true)).toBe(
      false,
    );
  });
});
