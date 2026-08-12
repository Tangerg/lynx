import { getContainer } from "@/main/container";
import type { CreateScheduleRequest, Schedule } from "@/rpc";
import { configureScheduleGateway } from "../application/ports/scheduleGateway";
import type { ScheduleGateway } from "../application/ports/scheduleGateway";
import type { ScheduleConfig, ScheduleConfigInput } from "../application/scheduleConfig";

function scheduleInput(input: ScheduleConfigInput): CreateScheduleRequest {
  return {
    title: input.title,
    instructions: input.instructions,
    ...(input.cwd ? { workspace: { path: input.cwd } } : {}),
    cron: input.cron,
  };
}

function scheduleConfig(schedule: Schedule): ScheduleConfig {
  const { workspace, ...config } = schedule;
  return {
    ...config,
    ...(workspace ? { cwd: workspace.path } : {}),
  };
}

const gateway: ScheduleGateway = {
  async create(input) {
    return scheduleConfig(await getContainer().client().schedules.create(scheduleInput(input)));
  },
  async update(input) {
    return scheduleConfig(
      await getContainer()
        .client()
        .schedules.update({
          ...scheduleInput(input),
          ...(input.cwd ? {} : { workspaceMode: "default" }),
          id: input.id,
          expectedRevision: input.revision,
          enabled: input.enabled,
        }),
    );
  },
  async setEnabled(schedule, enabled) {
    return scheduleConfig(
      await getContainer().client().schedules.update({
        id: schedule.id,
        expectedRevision: schedule.revision,
        enabled,
      }),
    );
  },
  async remove(id) {
    await getContainer().client().schedules.delete(id);
  },
  async runNow(id) {
    const run = await getContainer().client().schedules.runNow(id);
    return { sessionId: run.sessionId, runId: run.runId };
  },
};

export function installScheduleGateway(): () => void {
  return configureScheduleGateway(gateway);
}
