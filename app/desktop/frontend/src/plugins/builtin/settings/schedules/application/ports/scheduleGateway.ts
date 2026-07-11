import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { ScheduleConfig, ScheduleConfigInput } from "../scheduleConfig";

export interface ScheduleUpdateInput extends ScheduleConfigInput {
  id: string;
  enabled: boolean;
}

export interface ScheduleGateway {
  create(input: ScheduleConfigInput): Promise<ScheduleConfig>;
  update(input: ScheduleUpdateInput): Promise<ScheduleConfig>;
  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<void>;
  remove(id: string): Promise<void>;
  runNow(id: string): Promise<void>;
}

const port = createSingletonPort<ScheduleGateway>("Schedule gateway is not configured");

export const configureScheduleGateway = port.configure;
export const scheduleGateway = port.get;
