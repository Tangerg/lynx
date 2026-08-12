import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient, Schedule } from "@/rpc";
import { scheduleGateway } from "../application/ports/scheduleGateway";
import { installScheduleGateway } from "./runtimeScheduleGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

function schedule(workspace?: { path: string }): Schedule {
  return {
    id: "schedule-1",
    title: "Review",
    instructions: "Review changes",
    ...(workspace ? { workspace } : {}),
    cron: "0 9 * * 1",
    enabled: true,
    createdAt: "2026-08-12T00:00:00Z",
    revision: 1,
  };
}

describe("runtimeScheduleGateway", () => {
  it("omits workspace when a new schedule deliberately uses the Runtime default", async () => {
    const create = vi.fn().mockResolvedValue(schedule());
    setContainer({ client: () => ({ schedules: { create } }) as unknown as LyraClient });
    uninstall = installScheduleGateway();

    await scheduleGateway().create({
      title: "Review",
      instructions: "Review changes",
      cwd: "",
      cron: "0 9 * * 1",
    });

    expect(create).toHaveBeenCalledWith({
      title: "Review",
      instructions: "Review changes",
      cron: "0 9 * * 1",
    });
  });

  it("uses the explicit Runtime-default mode when an edit clears a binding", async () => {
    const update = vi.fn().mockResolvedValue(schedule());
    setContainer({ client: () => ({ schedules: { update } }) as unknown as LyraClient });
    uninstall = installScheduleGateway();

    await scheduleGateway().update({
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    });

    expect(update).toHaveBeenCalledWith({
      id: "schedule-1",
      expectedRevision: 7,
      title: "Review",
      instructions: "Review changes",
      workspaceMode: "default",
      cron: "0 9 * * 1",
      enabled: true,
    });
  });

  it("sends a valid workspace ref when an edit sets an explicit binding", async () => {
    const update = vi.fn().mockResolvedValue(schedule({ path: "/workspace" }));
    setContainer({ client: () => ({ schedules: { update } }) as unknown as LyraClient });
    uninstall = installScheduleGateway();

    await scheduleGateway().update({
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "/workspace",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    });

    expect(update).toHaveBeenCalledWith({
      id: "schedule-1",
      expectedRevision: 7,
      title: "Review",
      instructions: "Review changes",
      workspace: { path: "/workspace" },
      cron: "0 9 * * 1",
      enabled: true,
    });
  });

  it("preserves the launched session and run identities from run-now", async () => {
    const runNow = vi.fn().mockResolvedValue({ sessionId: "ses_scheduled", runId: "run_1" });
    setContainer({ client: () => ({ schedules: { runNow } }) as unknown as LyraClient });
    uninstall = installScheduleGateway();

    await expect(scheduleGateway().runNow("schedule-1")).resolves.toEqual({
      sessionId: "ses_scheduled",
      runId: "run_1",
    });
    expect(runNow).toHaveBeenCalledWith("schedule-1");
  });

  it("preserves the authoritative revision returned by an enablement change", async () => {
    const updated = { ...schedule(), enabled: false, revision: 8 };
    const update = vi.fn().mockResolvedValue(updated);
    setContainer({ client: () => ({ schedules: { update } }) as unknown as LyraClient });
    uninstall = installScheduleGateway();

    await expect(scheduleGateway().setEnabled(schedule(), false)).resolves.toMatchObject({
      enabled: false,
      revision: 8,
    });
  });
});
