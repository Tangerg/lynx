import { queryClient } from "@/lib/queryClient";
import type { ProviderGateway, ProviderTestOutcome, ProviderUpdate } from "./ports/providerGateway";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "./providerQueries";

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
  readonly #retirement: Promise<void>;
  readonly #retiredError = new ProviderMutationRetiredError();
  #signalRetirement!: () => void;
  readonly #tails = new Map<string, Promise<void>>();
  #retired = false;

  constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
    this.#retirement = new Promise<void>((resolve) => {
      this.#signalRetirement = resolve;
    });
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
      repair: [EMBEDDING_ROLE_KEY, CODEBASE_STATUS_KEY],
    });
  }

  testProvider(provider: string): Promise<ProviderTestOutcome> {
    return this.#execute(() => this.#gateway.testProvider(provider));
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#signalRetirement();
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
    return Promise.race([
      operation,
      this.#retirement.then(() => {
        throw this.#retiredError;
      }),
    ]);
  }

  #assertCurrent(): void {
    if (this.#retired) throw this.#retiredError;
  }
}

/** Owns Provider commands and their cache projection for one exact Plugin Host
 * and Runtime generation. */
export class ProviderMutationOwner {
  static #active: ProviderMutationOwner | null = null;

  readonly #gateway: ProviderGateway;
  #generation: ProviderMutationGeneration;
  #disposed = false;

  private constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
    this.#generation = new ProviderMutationGeneration(gateway);
  }

  static install(gateway: ProviderGateway): ProviderMutationOwner {
    const predecessor = ProviderMutationOwner.#active;
    const owner = new ProviderMutationOwner(gateway);
    ProviderMutationOwner.#active = owner;
    predecessor?.dispose();
    return owner;
  }

  static current(): ProviderMutationOwner {
    const owner = ProviderMutationOwner.#active;
    if (!owner || owner.#disposed) throw new Error("Provider mutation owner is not installed");
    return owner;
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
    if (this.#disposed || ProviderMutationOwner.#active !== this) return;
    const predecessor = this.#generation;
    this.#generation = new ProviderMutationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (ProviderMutationOwner.#active === this) ProviderMutationOwner.#active = null;
  }
}

export function providerMutationWasRetired(error: unknown): boolean {
  return error instanceof ProviderMutationRetiredError;
}

function commitProviderSaved(saved: ProviderConfiguration): void {
  queryClient.setQueryData<ProviderConfiguration[]>([PROVIDERS_KEY], (current) =>
    current?.map((provider) => (provider.id === saved.id ? saved : provider)),
  );
}
