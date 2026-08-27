import { getContainer } from "@/main/container";
import type { CreateScheduleRequest, ScopeAppClient, Schedule } from "@/rpc";
import { ScheduleMutationOwner, type ScheduleGateway } from "../application/scheduleCommands";
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

function runtimeScheduleGateway(client: ScopeAppClient): ScheduleGateway {
  return {
    async create(input) {
      return scheduleConfig(await client.schedules.create(scheduleInput(input)));
    },
    async update(input) {
      return scheduleConfig(
        await client.schedules.update({
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
        await client.schedules.update({
          id: schedule.id,
          expectedRevision: schedule.revision,
          enabled,
        }),
      );
    },
    async remove(id) {
      await client.schedules.delete(id);
    },
    async runNow(id) {
      const run = await client.schedules.runNow(id);
      return { sessionId: run.sessionId, runId: run.runId };
    },
  };
}

export function installScheduleGateway() {
  const owner = ScheduleMutationOwner.install(runtimeScheduleGateway(getContainer().client()));
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose() {
      owner.dispose();
    },
  };
}
