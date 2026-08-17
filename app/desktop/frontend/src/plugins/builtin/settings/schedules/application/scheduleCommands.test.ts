import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  runScheduleNow,
  ScheduleMutationOwner,
  setScheduleEnabled,
  type ScheduleGateway,
  updateSchedule,
} from "./scheduleCommands";
import { SCHEDULES_KEY } from "./scheduleQueries";

const { selectAgentSession } = vi.hoisted(() => ({ selectAgentSession: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));

let owner: ScheduleMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
  vi.restoreAllMocks();
});

describe("schedule commands", () => {
  it("returns the launched identity after invalidating schedule state", async () => {
    const run = { sessionId: "ses_scheduled", runId: "run_1" };
    const runNow = vi.fn().mockResolvedValue(run);
    owner = ScheduleMutationOwner.install({ runNow } as unknown as ScheduleGateway);
    queryClient.setQueryData([SCHEDULES_KEY], []);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    await expect(runScheduleNow("schedule-1")).resolves.toEqual(run);

    expect(runNow).toHaveBeenCalledWith("schedule-1");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [SCHEDULES_KEY] });
    expect(selectAgentSession).toHaveBeenCalledWith("ses_scheduled");
  });

  it("commits the authoritative enablement revision before revalidation", async () => {
    const current = {
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "/repo",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    };
    const updated = { ...current, enabled: false, revision: 8 };
    const setEnabled = vi.fn().mockResolvedValue(updated);
    owner = ScheduleMutationOwner.install({ setEnabled } as unknown as ScheduleGateway);
    queryClient.setQueryData([SCHEDULES_KEY], [current]);

    await expect(setScheduleEnabled(current, false)).resolves.toEqual(updated);

    expect(queryClient.getQueryData([SCHEDULES_KEY])).toEqual([updated]);
  });

  it("serializes same-schedule intents and rebases them on the accepted revision", async () => {
    const current = {
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "/repo",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    };
    const disabled = { ...current, enabled: false, revision: 8 };
    const first = deferred<typeof disabled>();
    const setEnabled = vi
      .fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce((schedule, enabled) =>
        Promise.resolve({ ...schedule, enabled, revision: schedule.revision + 1 }),
      );
    owner = ScheduleMutationOwner.install({ setEnabled } as unknown as ScheduleGateway);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([SCHEDULES_KEY], [current]);

    const disable = setScheduleEnabled(current, false);
    const enable = setScheduleEnabled(current, true);
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalled());
    const callsBeforeFirstSettlement = setEnabled.mock.calls.length;

    first.resolve(disabled);
    await expect(disable).resolves.toEqual(disabled);
    await expect(enable).resolves.toMatchObject({ enabled: true, revision: 9 });

    expect(callsBeforeFirstSettlement).toBe(1);
    expect(setEnabled.mock.calls[1]).toEqual([disabled, true]);
  });

  it("preserves accepted enablement when a queued edit rebases its revision", async () => {
    const current = {
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cwd: "/repo",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    };
    const disabled = { ...current, enabled: false, revision: 8 };
    const first = deferred<typeof disabled>();
    const setEnabled = vi.fn(() => first.promise);
    const update = vi.fn((input) => Promise.resolve({ ...input, revision: input.revision + 1 }));
    owner = ScheduleMutationOwner.install({ setEnabled, update } as unknown as ScheduleGateway);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([SCHEDULES_KEY], [current]);

    const disable = setScheduleEnabled(current, false);
    const edit = updateSchedule({
      ...current,
      title: "Review weekly",
      cwd: current.cwd,
    });
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledOnce());
    expect(update).not.toHaveBeenCalled();

    first.resolve(disabled);
    await expect(disable).resolves.toEqual(disabled);
    await expect(edit).resolves.toMatchObject({
      title: "Review weekly",
      enabled: false,
      revision: 9,
    });
    expect(update).toHaveBeenCalledWith({
      ...current,
      title: "Review weekly",
      cwd: current.cwd,
      enabled: false,
      revision: 8,
    });
  });

  it("does not block a different schedule behind an in-flight mutation", async () => {
    const current = {
      id: "schedule-1",
      title: "Review",
      instructions: "Review changes",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    };
    const slow = deferred<typeof current>();
    const run = { sessionId: "ses_other", runId: "run_other" };
    owner = ScheduleMutationOwner.install({
      setEnabled: vi.fn(() => slow.promise),
      runNow: vi.fn().mockResolvedValue(run),
    } as unknown as ScheduleGateway);
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const mutation = setScheduleEnabled(current, false);
    await expect(runScheduleNow("schedule-2")).resolves.toEqual(run);

    slow.resolve({ ...current, enabled: false, revision: 8 });
    await expect(mutation).resolves.toMatchObject({ enabled: false, revision: 8 });
  });

  it("navigates an accepted run even when schedule cache repair fails", async () => {
    const run = { sessionId: "ses_scheduled", runId: "run_1" };
    owner = ScheduleMutationOwner.install({
      runNow: vi.fn().mockResolvedValue(run),
    } as unknown as ScheduleGateway);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("cache unavailable"));

    await expect(runScheduleNow("schedule-1")).resolves.toEqual(run);
    expect(selectAgentSession).toHaveBeenCalledWith("ses_scheduled");
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
