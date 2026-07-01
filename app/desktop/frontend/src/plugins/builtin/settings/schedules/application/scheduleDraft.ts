import type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

export const CRON_PRESETS: Array<{ key: string; cron: string }> = [
  { key: "schedules.preset.hourly", cron: "0 * * * *" },
  { key: "schedules.preset.daily", cron: "0 9 * * *" },
  { key: "schedules.preset.weekdays", cron: "0 9 * * 1-5" },
  { key: "schedules.preset.weekly", cron: "0 9 * * 1" },
];

export interface ScheduleDraft {
  title: string;
  prompt: string;
  cron: string;
  cwd: string;
}

export function initialScheduleDraft(
  schedule?: ScheduleConfig,
  defaultCwd?: string,
): ScheduleDraft {
  return {
    title: schedule?.title ?? "",
    prompt: schedule?.prompt ?? "",
    cron: schedule?.cron ?? "0 9 * * 1-5",
    cwd: schedule?.cwd ?? defaultCwd ?? "",
  };
}

export function canSaveScheduleDraft(draft: ScheduleDraft, busy: boolean): boolean {
  return draft.prompt.trim() !== "" && draft.cron.trim() !== "" && !busy;
}

export function scheduleInputFromDraft(draft: ScheduleDraft): ScheduleConfigInput {
  return {
    title: draft.title.trim(),
    prompt: draft.prompt.trim(),
    cwd: draft.cwd.trim(),
    cron: draft.cron.trim(),
  };
}
