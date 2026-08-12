import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { configureScheduleGateway, type ScheduleGateway } from "./ports/scheduleGateway";
import { runScheduleNow, setScheduleEnabled } from "./scheduleCommands";
import { SCHEDULES_KEY } from "./scheduleQueries";

const { selectAgentSession } = vi.hoisted(() => ({ selectAgentSession: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
  vi.restoreAllMocks();
});

describe("schedule commands", () => {
  it("returns the launched identity after invalidating schedule state", async () => {
    const run = { sessionId: "ses_scheduled", runId: "run_1" };
    const runNow = vi.fn().mockResolvedValue(run);
    uninstall = configureScheduleGateway({ runNow } as unknown as ScheduleGateway);
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
    uninstall = configureScheduleGateway({ setEnabled } as unknown as ScheduleGateway);
    queryClient.setQueryData([SCHEDULES_KEY], [current]);

    await expect(setScheduleEnabled(current, false)).resolves.toEqual(updated);

    expect(queryClient.getQueryData([SCHEDULES_KEY])).toEqual([updated]);
  });
});
