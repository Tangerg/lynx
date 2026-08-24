import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { ProviderGateway, ProviderTestOutcome, ProviderUpdate } from "./ports/providerGateway";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";
import { EMBEDDING_ROLE_KEY, MODELS_KEY, PROVIDERS_KEY, UTILITY_ROLE_KEY } from "./providerQueries";

class ProviderMutationRetiredError extends Error {
  override readonly name = "ProviderMutationRetiredError";

  constructor() {
    super("provider_mutation_generation_retired");
  }
}

interface ProviderMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
  repair: readonly string[];
}

class ProviderMutationGeneration {
  readonly #gateway: ProviderGateway;
  readonly #retiredError = new ProviderMutationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();

  constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
  }

  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
    return this.#run(`provider\u0000${input.provider}`, {
      execute: () => this.#gateway.updateProvider(input),
      commit: commitProviderSaved,
      repair: [PROVIDERS_KEY, MODELS_KEY],
    });
  }

  setUtilityRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#run("utility-role", {
      execute: () => this.#gateway.setUtilityRole(role),
      commit: (saved) => queryClient.setQueryData([UTILITY_ROLE_KEY], saved),
      repair: [UTILITY_ROLE_KEY],
    });
  }

  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#run("embedding-role", {
      execute: () => this.#gateway.setEmbeddingRole(role),
      commit: (saved) => queryClient.setQueryData([EMBEDDING_ROLE_KEY], saved),
      repair: [EMBEDDING_ROLE_KEY],
    });
  }

  testProvider(provider: string): Promise<ProviderTestOutcome> {
    return this.#execute(() => this.#gateway.testProvider(provider));
  }

  retire(): void {
    this.#cohort.retire();
    this.#tails.clear();
  }

  #run<T>(identity: string, mutation: ProviderMutation<T>): Promise<T> {
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(async () => {
      const value = await this.#execute(mutation.execute);
      mutation.commit(value);
      await this.#repairProjection(mutation.repair);
      this.#assertCurrent();
      return value;
    });
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

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  async #repairProjection(keys: readonly string[]): Promise<void> {
    try {
      await Promise.all(
        keys.map((key) =>
          this.#settle(queryClient.invalidateQueries({ queryKey: [key] })).then(() => undefined),
        ),
      );
    } catch (error) {
      if (error === this.#retiredError) throw error;
      // The command response already committed its authoritative resource.
      // Runtime events and the next read remain the projection repair path.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns Provider commands and their cache projection for one exact Plugin Host
 * and Runtime generation. */
export class ProviderMutationOwner {
  static #materialGeneration = 0;
  static readonly #materialListeners = new Set<() => void>();

  readonly #gateway: ProviderGateway;
  #generation: ProviderMutationGeneration;
  #disposed = false;

  private constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
    this.#generation = new ProviderMutationGeneration(gateway);
  }

  static install(gateway: ProviderGateway): ProviderMutationOwner {
    const owner = new ProviderMutationOwner(gateway);
    providerMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    ProviderMutationOwner.#advanceMaterialGeneration();
    return owner;
  }

  static current(): ProviderMutationOwner {
    const owner = providerMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Provider mutation owner is not installed");
    return owner;
  }

  static materialGeneration(): number {
    return ProviderMutationOwner.#materialGeneration;
  }

  static subscribeMaterialGeneration(listener: () => void): () => void {
    ProviderMutationOwner.#materialListeners.add(listener);
    return () => ProviderMutationOwner.#materialListeners.delete(listener);
  }

  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
    return this.#generation.updateProvider(input);
  }

  setUtilityRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#generation.setUtilityRole(role);
  }

  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#generation.setEmbeddingRole(role);
  }

  testProvider(provider: string): Promise<ProviderTestOutcome> {
    return this.#generation.testProvider(provider);
  }

  errorMessage(error: unknown): string | undefined {
    return this.#gateway.errorMessage(error);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !providerMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new ProviderMutationGeneration(this.#gateway);
    predecessor.retire();
    ProviderMutationOwner.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (providerMutationPublication.withdraw(this)) {
      ProviderMutationOwner.#advanceMaterialGeneration();
    }
  }

  static #advanceMaterialGeneration(): void {
    ProviderMutationOwner.#materialGeneration += 1;
    for (const listener of ProviderMutationOwner.#materialListeners) listener();
  }
}

const providerMutationPublication = createPublicationSlot<ProviderMutationOwner>();

export function providerMutationWasRetired(error: unknown): boolean {
  return error instanceof ProviderMutationRetiredError;
}

function commitProviderSaved(saved: ProviderConfiguration): void {
  queryClient.setQueryData<ProviderConfiguration[]>([PROVIDERS_KEY], (current) =>
    current?.map((provider) => (provider.id === saved.id ? saved : provider)),
  );
}
