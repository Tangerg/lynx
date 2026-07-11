import { getContainer } from "@/main/container";
import type { ScheduleInput } from "@/rpc";
import { configureScheduleGateway } from "../application/ports/scheduleGateway";
import type { ScheduleGateway } from "../application/ports/scheduleGateway";
import type { ScheduleConfigInput } from "../application/scheduleConfig";

function scheduleInput(input: ScheduleConfigInput): ScheduleInput {
  return {
    title: input.title,
    prompt: input.prompt,
    cwd: input.cwd,
    cron: input.cron,
  };
}

const gateway: ScheduleGateway = {
  async create(input) {
    return getContainer().client().schedules.create(scheduleInput(input));
  },
  async update(input) {
    return getContainer()
      .client()
      .schedules.update({ ...scheduleInput(input), id: input.id, enabled: input.enabled });
  },
  async setEnabled(schedule, enabled) {
    await getContainer().client().schedules.update({
      id: schedule.id,
      title: schedule.title,
      prompt: schedule.prompt,
      cwd: schedule.cwd,
      provider: schedule.provider,
      model: schedule.model,
      cron: schedule.cron,
      enabled,
    });
  },
  async remove(id) {
    await getContainer().client().schedules.delete(id);
  },
  async runNow(id) {
    await getContainer().client().schedules.runNow(id);
  },
};

export function installScheduleGateway(): () => void {
  return configureScheduleGateway(gateway);
}
