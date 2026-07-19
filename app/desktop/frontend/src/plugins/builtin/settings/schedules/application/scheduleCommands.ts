// Scheduled-run mutations. The counterpart read is useSchedules(); every
// mutator invalidates it so the pane re-reads the new set and recomputed
// nextRunAt.

import { SCHEDULES_KEY, useSchedules } from "./scheduleQueries";
import { queryClient } from "@/lib/data/queryClient";
import type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";
import { scheduleGateway } from "./ports/scheduleGateway";
export type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

export function useScheduleConfigs() {
  return useSchedules();
}

function invalidate(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: [SCHEDULES_KEY] }).then(() => undefined);
}

export async function createSchedule(input: ScheduleConfigInput): Promise<ScheduleConfig> {
  const s = await scheduleGateway().create(input);
  await invalidate();
  return s;
}

export async function updateSchedule(
  input: ScheduleConfigInput & { id: string; enabled: boolean; revision: number },
): Promise<ScheduleConfig> {
  const s = await scheduleGateway().update(input);
  await invalidate();
  return s;
}

// setScheduleEnabled flips just the enablement without dropping the schedule's
// other persisted fields.
export async function setScheduleEnabled(s: ScheduleConfig, enabled: boolean): Promise<void> {
  await scheduleGateway().setEnabled(s, enabled);
  await invalidate();
}

export async function deleteSchedule(id: string): Promise<void> {
  await scheduleGateway().remove(id);
  await invalidate();
}

// runScheduleNow fires the schedule immediately. Re-read the schedules so the
// row's lastRunAt updates when the runtime reports the run.
export async function runScheduleNow(id: string): Promise<void> {
  await scheduleGateway().runNow(id);
  await invalidate();
}
