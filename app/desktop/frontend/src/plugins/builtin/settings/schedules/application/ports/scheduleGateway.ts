import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { ScheduleConfig, ScheduleConfigInput, ScheduledRunIdentity } from "../scheduleConfig";

export interface ScheduleUpdateInput extends ScheduleConfigInput {
  id: string;
  enabled: boolean;
  revision: number;
}

export interface ScheduleGateway {
  create(input: ScheduleConfigInput): Promise<ScheduleConfig>;
  update(input: ScheduleUpdateInput): Promise<ScheduleConfig>;
  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<ScheduleConfig>;
  remove(id: string): Promise<void>;
  runNow(id: string): Promise<ScheduledRunIdentity>;
}

const port = createSingletonPort<ScheduleGateway>("Schedule gateway is not configured");

export const configureScheduleGateway = port.configure;
export const scheduleGateway = port.get;
