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

let port: ScheduleGateway | null = null;

export function configureScheduleGateway(next: ScheduleGateway): void {
  port = next;
}

export function scheduleGateway(): ScheduleGateway {
  if (!port) throw new Error("Schedule gateway is not configured");
  return port;
}
