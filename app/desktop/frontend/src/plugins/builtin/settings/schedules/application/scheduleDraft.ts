import type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

export const CRON_PRESETS: Array<{ key: string; cron: string }> = [
  { key: "schedules.preset.hourly", cron: "0 * * * *" },
  { key: "schedules.preset.daily", cron: "0 9 * * *" },
  { key: "schedules.preset.weekdays", cron: "0 9 * * 1-5" },
  { key: "schedules.preset.weekly", cron: "0 9 * * 1" },
];

export interface ScheduleDraft {
  title: string;
  instructions: string;
  cron: string;
  cwd: string;
}

export function initialScheduleDraft(
  schedule?: ScheduleConfig,
  defaultCwd?: string,
): ScheduleDraft {
  return {
    title: schedule?.title ?? "",
    instructions: schedule?.instructions ?? "",
    cron: schedule?.cron ?? "0 9 * * 1-5",
    // A persisted schedule with no workspace deliberately targets the Runtime's
    // default. Only a NEW schedule inherits the currently selected project;
    // applying that convenience to edits silently relocates old schedules.
    cwd: schedule ? (schedule.cwd ?? "") : (defaultCwd ?? ""),
  };
}

export function canSaveScheduleDraft(draft: ScheduleDraft, busy: boolean): boolean {
  return draft.instructions.trim() !== "" && draft.cron.trim() !== "" && !busy;
}

export function scheduleInputFromDraft(draft: ScheduleDraft): ScheduleConfigInput {
  return {
    title: draft.title.trim(),
    instructions: draft.instructions.trim(),
    cwd: draft.cwd.trim(),
    cron: draft.cron.trim(),
  };
}
