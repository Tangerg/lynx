import { createDataQuery } from "@/lib/data/dataQuery";
import type { ScheduleConfig } from "./scheduleConfig";

export const SCHEDULES_KEY = "schedules";
export const useSchedules = createDataQuery<ScheduleConfig[]>(SCHEDULES_KEY);
