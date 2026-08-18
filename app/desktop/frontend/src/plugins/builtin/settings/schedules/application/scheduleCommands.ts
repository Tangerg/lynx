import { SCHEDULES_KEY, useSchedules } from "./scheduleQueries";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { ScheduleConfig, ScheduleConfigInput, ScheduledRunIdentity } from "./scheduleConfig";
import { selectAgentSession } from "@/plugins/builtin/agent/public/session";
export type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

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

class ScheduleMutationRetiredError extends Error {
  override readonly name = "ScheduleMutationRetiredError";

  constructor() {
    super("schedule_mutation_generation_retired");
  }
}

class ScheduleMutationGeneration {
  readonly #gateway: ScheduleGateway;
  readonly #retiredError = new ScheduleMutationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();
  readonly #accepted = new Map<string, ScheduleConfig>();

  constructor(gateway: ScheduleGateway) {
    this.#gateway = gateway;
  }

  create(input: ScheduleConfigInput): Promise<ScheduleConfig> {
    return this.#executeMutation(
      () => this.#gateway.create(input),
      (saved) => this.#commitSaved(saved),
    );
  }

  update(input: ScheduleUpdateInput): Promise<ScheduleConfig> {
    return this.#run(
      input.id,
      () => {
        const basis = this.#latest(input.id, input);
        return this.#gateway.update({
          ...input,
          enabled: basis.enabled,
          revision: basis.revision,
        });
      },
      (saved) => this.#commitSaved(saved),
    );
  }

  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<ScheduleConfig> {
    return this.#run(
      schedule.id,
      () => this.#gateway.setEnabled(this.#latest(schedule.id, schedule), enabled),
      (saved) => this.#commitSaved(saved),
    );
  }

  remove(id: string): Promise<void> {
    return this.#run(
      id,
      () => this.#gateway.remove(id),
      () => {
        this.#accepted.delete(id);
        removeSchedule(id);
      },
    );
  }

  runNow(id: string): Promise<ScheduledRunIdentity> {
    return this.#run(
      id,
      () => this.#gateway.runNow(id),
      () => undefined,
      (run) => selectAgentSession(run.sessionId),
    );
  }

  retire(): void {
    this.#cohort.retire();
    this.#tails.clear();
    this.#accepted.clear();
  }

  #run<T>(
    identity: string,
    execute: () => Promise<T>,
    commit: (value: T) => void,
    afterRepair?: (value: T) => void,
  ): Promise<T> {
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(() =>
      this.#executeMutation(execute, commit, afterRepair),
    );
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  async #executeMutation<T>(
    execute: () => Promise<T>,
    commit: (value: T) => void,
    afterRepair?: (value: T) => void,
  ): Promise<T> {
    this.#assertCurrent();
    let value: T;
    try {
      value = await this.#settle(execute());
    } catch (error) {
      if (error === this.#retiredError) throw error;
      await this.#repairProjection();
      this.#assertCurrent();
      throw error;
    }
    this.#assertCurrent();
    commit(value);
    await this.#repairProjection();
    this.#assertCurrent();
    afterRepair?.(value);
    return value;
  }

  async #repairProjection(): Promise<void> {
    try {
      await this.#settle(queryClient.invalidateQueries({ queryKey: [SCHEDULES_KEY] }));
    } catch (error) {
      if (error === this.#retiredError) throw error;
      // An accepted response remains authoritative. Schedule events and the
      // next read retain the projection repair path.
    }
  }

  #commitSaved(saved: ScheduleConfig): void {
    this.#accepted.set(saved.id, saved);
    commitScheduleSaved(saved);
  }

  #latest(identity: string, fallback: ScheduleConfig | ScheduleUpdateInput): ScheduleConfig {
    const cached = queryClient
      .getQueryData<ScheduleConfig[]>([SCHEDULES_KEY])
      ?.find((schedule) => schedule.id === identity);
    const candidates = [fallback, cached, this.#accepted.get(identity)].filter(
      (candidate): candidate is ScheduleConfig | ScheduleUpdateInput => candidate !== undefined,
    );
    return candidates.reduce((latest, candidate) =>
      candidate.revision > latest.revision ? candidate : latest,
    ) as ScheduleConfig;
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns schedule commands, revisions, cache projection, and run navigation for one generation. */
export class ScheduleMutationOwner {
  readonly #gateway: ScheduleGateway;
  #generation: ScheduleMutationGeneration;
  #disposed = false;

  private constructor(gateway: ScheduleGateway) {
    this.#gateway = gateway;
    this.#generation = new ScheduleMutationGeneration(gateway);
  }

  static install(gateway: ScheduleGateway): ScheduleMutationOwner {
    const owner = new ScheduleMutationOwner(gateway);
    scheduleMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): ScheduleMutationOwner {
    const owner = scheduleMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Schedule mutation owner is not installed");
    return owner;
  }

  create(input: ScheduleConfigInput): Promise<ScheduleConfig> {
    return this.#generation.create(input);
  }

  update(input: ScheduleUpdateInput): Promise<ScheduleConfig> {
    return this.#generation.update(input);
  }

  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<ScheduleConfig> {
    return this.#generation.setEnabled(schedule, enabled);
  }

  remove(id: string): Promise<void> {
    return this.#generation.remove(id);
  }

  runNow(id: string): Promise<ScheduledRunIdentity> {
    return this.#generation.runNow(id);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !scheduleMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new ScheduleMutationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    scheduleMutationPublication.withdraw(this);
  }
}

const scheduleMutationPublication = createPublicationSlot<ScheduleMutationOwner>();

export function useScheduleConfigs() {
  return useSchedules();
}

export async function createSchedule(input: ScheduleConfigInput): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().create(input);
}

export async function updateSchedule(
  input: ScheduleConfigInput & { id: string; enabled: boolean; revision: number },
): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().update(input);
}

// setScheduleEnabled flips just the enablement without dropping the schedule's
// other persisted fields.
export async function setScheduleEnabled(
  s: ScheduleConfig,
  enabled: boolean,
): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().setEnabled(s, enabled);
}

export async function deleteSchedule(id: string): Promise<void> {
  return ScheduleMutationOwner.current().remove(id);
}

// runScheduleNow fires the schedule immediately. Re-read the schedules so the
// row's lastRunAt updates when the runtime reports the run.
export async function runScheduleNow(id: string): Promise<ScheduledRunIdentity> {
  return ScheduleMutationOwner.current().runNow(id);
}

export function scheduleMutationWasRetired(error: unknown): boolean {
  return error instanceof ScheduleMutationRetiredError;
}

function commitScheduleSaved(saved: ScheduleConfig): void {
  queryClient.setQueryData<ScheduleConfig[]>([SCHEDULES_KEY], (current) => {
    if (!current) return current;
    const index = current.findIndex((schedule) => schedule.id === saved.id);
    if (index < 0) return [...current, saved];
    return current.map((schedule) => (schedule.id === saved.id ? saved : schedule));
  });
}

function removeSchedule(id: string): void {
  queryClient.setQueryData<ScheduleConfig[]>([SCHEDULES_KEY], (current) =>
    current?.filter((schedule) => schedule.id !== id),
  );
}
