// Scheduled-run mutations (schedules.create / update / delete / runNow). The
// counterpart read is useSchedules(); every mutator invalidates it so the pane
// re-reads the new set and recomputed nextRunAt.

import type { ScheduleInput } from "@/rpc";
import { SCHEDULES_KEY, useSchedules } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";
import type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";
export type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

export function useScheduleConfigs() {
  return useSchedules();
}

function invalidate(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: [SCHEDULES_KEY] }).then(() => undefined);
}

function scheduleInput(input: ScheduleConfigInput): ScheduleInput {
  return {
    title: input.title,
    prompt: input.prompt,
    cwd: input.cwd,
    cron: input.cron,
  };
}

export async function createSchedule(input: ScheduleConfigInput): Promise<ScheduleConfig> {
  const s = await getContainer().client().schedules.create(scheduleInput(input));
  await invalidate();
  return s;
}

export async function updateSchedule(
  input: ScheduleConfigInput & { id: string; enabled: boolean },
): Promise<ScheduleConfig> {
  const s = await getContainer()
    .client()
    .schedules.update({ ...scheduleInput(input), id: input.id, enabled: input.enabled });
  await invalidate();
  return s;
}

// setScheduleEnabled flips just the enablement, re-sending the schedule's other
// fields verbatim (update is a full-replace) so the toggle never drops them.
export async function setScheduleEnabled(s: ScheduleConfig, enabled: boolean): Promise<void> {
  await getContainer().client().schedules.update({
    id: s.id,
    title: s.title,
    prompt: s.prompt,
    cwd: s.cwd,
    provider: s.provider,
    model: s.model,
    cron: s.cron,
    enabled,
  });
  await invalidate();
}

export async function deleteSchedule(id: string): Promise<void> {
  await getContainer().client().schedules.delete(id);
  await invalidate();
}

// runScheduleNow fires the schedule immediately; the new session arrives via the
// schedules.fired workspace event (which refreshes the sidebar). Re-read the
// schedules so the row's lastRunAt updates.
export async function runScheduleNow(id: string): Promise<void> {
  await getContainer().client().schedules.runNow(id);
  await invalidate();
}
