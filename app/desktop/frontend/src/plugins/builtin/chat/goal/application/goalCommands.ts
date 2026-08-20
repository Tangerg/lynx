import { createPublicationSlot } from "@/lib/publicationSlot";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import {
  type GoalCommandsGateway,
  type GoalCommandReceipt,
  type StartGoalInput,
  type UpdateGoalInput,
} from "./ports/goalCommandsGateway";

export type GoalProjectionRepair = (sessionId: string) => Promise<unknown>;

export class GoalCommandSessionMismatchError extends Error {
  constructor(
    readonly expectedSessionId: string,
    readonly actualSessionId: string,
  ) {
    super();
    this.name = "GoalCommandSessionMismatchError";
  }
}

class GoalCommandGenerationRetiredError extends Error {
  override readonly name = "GoalCommandGenerationRetiredError";

  constructor() {
    super("goal_command_generation_retired");
  }
}

class GoalCommandGeneration {
  readonly #gateway: GoalCommandsGateway;
  readonly #repairProjection: GoalProjectionRepair;
  readonly #retiredError = new GoalCommandGenerationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();

  constructor(gateway: GoalCommandsGateway, repairProjection: GoalProjectionRepair) {
    this.#gateway = gateway;
    this.#repairProjection = repairProjection;
  }

  start(input: StartGoalInput): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.start(input));
  }

  update(input: UpdateGoalInput): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.update(input));
  }

  clear(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.clear(sessionId));
  }

  stop(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.stop(sessionId));
  }

  resume(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.resume(sessionId));
  }

  retire(): void {
    this.#cohort.retire();
    this.#tails.clear();
  }

  #run(sessionId: string, command: () => Promise<GoalCommandReceipt>): Promise<void> {
    const result = this.#settle(this.#tails.get(sessionId) ?? Promise.resolve()).then(() =>
      this.#mutate(sessionId, command),
    );
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(sessionId, settlement);
    void settlement.then(() => {
      if (this.#tails.get(sessionId) === settlement) this.#tails.delete(sessionId);
    });
    return result;
  }

  async #mutate(sessionId: string, command: () => Promise<GoalCommandReceipt>): Promise<void> {
    try {
      const committed = await this.#execute(command);
      if (committed.sessionId !== sessionId) {
        throw new GoalCommandSessionMismatchError(sessionId, committed.sessionId);
      }
    } catch (error) {
      if (this.#cohort.retired) throw error;
      // A command rejection may race Runtime's own loop progression, or its
      // success response may be lost after the transaction commits. Preserve the
      // command error for its caller while converging the standing Goal material.
      await this.#repairStandingProjection(sessionId);
      throw error;
    }
    // Mutation responses are point-in-time acknowledgements, not the standing
    // Goal read model. Keep the next local command behind the mounted Session's
    // authoritative material transaction so Goal cannot advance separately from
    // Plan/HITL/Run/Tool or accept a late independent query writer.
    await this.#repairStandingProjection(sessionId);
    this.#assertCurrent();
  }

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  async #repairStandingProjection(sessionId: string): Promise<void> {
    this.#assertCurrent();
    try {
      await this.#settle(this.#repairProjection(sessionId));
    } catch (error) {
      if (this.#cohort.retired) throw error;
      // A durable command receipt and an ambiguous command failure retain their
      // own meanings. Runtime events and the next read remain repair paths when
      // the standing projection itself cannot be fetched.
    }
  }

  #settle<T>(operation: PromiseLike<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns Goal intent ordering, command settlement and Session material repair for
 * one exact Plugin Host and Runtime generation. */
export class GoalCommandOwner {
  #generation: GoalCommandGeneration | null;
  readonly #repairProjection: GoalProjectionRepair;
  #disposed = false;

  private constructor(gateway: GoalCommandsGateway | null, repairProjection: GoalProjectionRepair) {
    this.#repairProjection = repairProjection;
    this.#generation = gateway ? new GoalCommandGeneration(gateway, repairProjection) : null;
  }

  static install(
    gateway: GoalCommandsGateway | null,
    repairProjection: GoalProjectionRepair,
  ): GoalCommandOwner {
    const owner = new GoalCommandOwner(gateway, repairProjection);
    goalCommandPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): GoalCommandOwner {
    const owner = goalCommandPublication.current();
    if (!owner || owner.#disposed) throw new Error("Goal command owner is not installed");
    return owner;
  }

  start(input: StartGoalInput): Promise<void> {
    return this.#currentGeneration().start(input);
  }

  update(input: UpdateGoalInput): Promise<void> {
    return this.#currentGeneration().update(input);
  }

  clear(sessionId: string): Promise<void> {
    return this.#currentGeneration().clear(sessionId);
  }

  stop(sessionId: string): Promise<void> {
    return this.#currentGeneration().stop(sessionId);
  }

  resume(sessionId: string): Promise<void> {
    return this.#currentGeneration().resume(sessionId);
  }

  replaceRuntimeGeneration(gateway: GoalCommandsGateway): boolean {
    if (this.#disposed || !goalCommandPublication.owns(this)) return false;
    const predecessor = this.#generation;
    this.#generation = new GoalCommandGeneration(gateway, this.#repairProjection);
    predecessor?.retire();
    return true;
  }

  retireRuntimeGeneration(): boolean {
    if (this.#disposed || !goalCommandPublication.owns(this)) return false;
    const predecessor = this.#generation;
    this.#generation = null;
    predecessor?.retire();
    return true;
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation?.retire();
    this.#generation = null;
    goalCommandPublication.withdraw(this);
  }

  #currentGeneration(): GoalCommandGeneration {
    if (this.#disposed || !goalCommandPublication.owns(this) || !this.#generation) {
      throw new GoalCommandGenerationRetiredError();
    }
    return this.#generation;
  }
}

const goalCommandPublication = createPublicationSlot<GoalCommandOwner>();

export async function startGoal(input: StartGoalInput): Promise<void> {
  await GoalCommandOwner.current().start(input);
}

export async function updateGoal(input: UpdateGoalInput): Promise<void> {
  await GoalCommandOwner.current().update(input);
}

export async function clearGoal(sessionId: string): Promise<void> {
  await GoalCommandOwner.current().clear(sessionId);
}

export async function stopGoal(sessionId: string): Promise<void> {
  await GoalCommandOwner.current().stop(sessionId);
}

export async function resumeGoal(sessionId: string): Promise<void> {
  await GoalCommandOwner.current().resume(sessionId);
}

export function goalCommandWasRetired(error: unknown): boolean {
  return error instanceof GoalCommandGenerationRetiredError;
}
