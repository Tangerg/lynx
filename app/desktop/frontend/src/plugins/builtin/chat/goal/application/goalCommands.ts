import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { GOAL_KEY } from "./goalQueries";
import {
  type GoalCommandsGateway,
  type GoalCommandReceipt,
  type StartGoalInput,
} from "./ports/goalCommandsGateway";

function invalidateGoal(sessionId: string): Promise<void> {
  return queryClient
    .invalidateQueries({ queryKey: [GOAL_KEY, { sessionId }], exact: true })
    .then(() => undefined);
}

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
  readonly #retiredError = new GoalCommandGenerationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();

  constructor(gateway: GoalCommandsGateway) {
    this.#gateway = gateway;
  }

  start(input: StartGoalInput): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.start(input));
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
      // A command rejection may be a revision/admission race with another client,
      // or a response lost after Runtime committed. Preserve the command error for
      // its caller while independently converging the standing Goal read model.
      await this.#repairProjection(sessionId);
      throw error;
    }
    // Mutation responses are point-in-time acknowledgements, not the standing
    // Goal read model. Only goals.get may write that model: a delayed response can
    // otherwise overwrite a newer autonomous or remote transition, and updatedAt
    // cannot establish authority across equal timestamps or a clock correction.
    // Keep the next local command behind this read so intent order and projection
    // order have one settlement boundary.
    await this.#repairProjection(sessionId);
    this.#assertCurrent();
  }

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  async #repairProjection(sessionId: string): Promise<void> {
    this.#assertCurrent();
    try {
      await this.#settle(invalidateGoal(sessionId));
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

/** Owns Goal intent ordering, command settlement and query repair for one exact
 * Plugin Host and Runtime generation. */
export class GoalCommandOwner {
  static #active: GoalCommandOwner | null = null;

  #generation: GoalCommandGeneration;
  #disposed = false;

  private constructor(gateway: GoalCommandsGateway) {
    this.#generation = new GoalCommandGeneration(gateway);
  }

  static install(gateway: GoalCommandsGateway): GoalCommandOwner {
    const predecessor = GoalCommandOwner.#active;
    const owner = new GoalCommandOwner(gateway);
    GoalCommandOwner.#active = owner;
    predecessor?.dispose();
    return owner;
  }

  static current(): GoalCommandOwner {
    const owner = GoalCommandOwner.#active;
    if (!owner || owner.#disposed) throw new Error("Goal command owner is not installed");
    return owner;
  }

  start(input: StartGoalInput): Promise<void> {
    return this.#generation.start(input);
  }

  stop(sessionId: string): Promise<void> {
    return this.#generation.stop(sessionId);
  }

  resume(sessionId: string): Promise<void> {
    return this.#generation.resume(sessionId);
  }

  replaceRuntimeGeneration(gateway: GoalCommandsGateway): boolean {
    if (this.#disposed || GoalCommandOwner.#active !== this) return false;
    const predecessor = this.#generation;
    this.#generation = new GoalCommandGeneration(gateway);
    predecessor.retire();
    return true;
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (GoalCommandOwner.#active === this) GoalCommandOwner.#active = null;
  }
}

export async function startGoal(input: StartGoalInput): Promise<void> {
  await GoalCommandOwner.current().start(input);
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
