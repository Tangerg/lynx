import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient, Schedule } from "@/rpc";
import {
  createSchedule,
  runScheduleNow,
  setScheduleEnabled,
  updateSchedule,
} from "../application/scheduleCommands";
import { SCHEDULES_KEY } from "../application/scheduleQueries";
import { installScheduleGateway } from "./runtimeScheduleGateway";

const { selectAgentSession } = vi.hoisted(() => ({ selectAgentSession: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));

let installation: ReturnType<typeof installScheduleGateway> | undefined;

afterEach(() => {
  installation?.dispose();
  installation = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
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
    installation = installScheduleGateway();

    await createSchedule({
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
    installation = installScheduleGateway();

    await updateSchedule({
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
    installation = installScheduleGateway();

    await updateSchedule({
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
    installation = installScheduleGateway();

    await expect(runScheduleNow("schedule-1")).resolves.toEqual({
      sessionId: "ses_scheduled",
      runId: "run_1",
    });
    expect(runNow).toHaveBeenCalledWith("schedule-1");
  });

  it("preserves the authoritative revision returned by an enablement change", async () => {
    const updated = { ...schedule(), enabled: false, revision: 8 };
    const update = vi.fn().mockResolvedValue(updated);
    setContainer({ client: () => ({ schedules: { update } }) as unknown as LyraClient });
    installation = installScheduleGateway();

    await expect(setScheduleEnabled(schedule(), false)).resolves.toMatchObject({
      enabled: false,
      revision: 8,
    });
  });

  it("does not navigate when a retired run-now response arrives after replacement", async () => {
    const retiredRun = deferred<{ sessionId: string; runId: string }>();
    const runNowRetired = vi.fn(() => retiredRun.promise);
    const runNowSuccessor = vi.fn().mockResolvedValue({
      sessionId: "ses_successor",
      runId: "run_successor",
    });
    setContainer({
      client: () => ({ schedules: { runNow: runNowRetired } }) as unknown as LyraClient,
    });
    const retiredInstallation = installScheduleGateway();
    const command = rejected(runScheduleNow("schedule-1"));
    await vi.waitFor(() => expect(runNowRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ schedules: { runNow: runNowSuccessor } }) as unknown as LyraClient,
    });
    const successorInstallation = installScheduleGateway();
    installation = {
      replaceRuntimeGeneration: () => successorInstallation.replaceRuntimeGeneration(),
      dispose() {
        successorInstallation.dispose();
        retiredInstallation.dispose();
      },
    };
    retiredRun.resolve({ sessionId: "ses_retired", runId: "run_retired" });

    await expect(command).resolves.toMatchObject({
      message: "schedule_mutation_generation_retired",
    });
    expect(runNowSuccessor).not.toHaveBeenCalled();
    expect(selectAgentSession).not.toHaveBeenCalled();
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
