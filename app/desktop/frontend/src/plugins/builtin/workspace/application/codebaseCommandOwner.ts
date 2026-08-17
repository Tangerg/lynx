import { queryClient } from "@/lib/queryClient";
import {
  CODEBASE_STATUS_KEY,
  commitCodebaseReindexStarted,
} from "@/plugins/builtin/settings/providers/public/queries";
import type {
  CodebaseGateway,
  CodebaseReindexOperation,
  CodebaseSearchHit,
} from "./ports/codebaseGateway";

class CodebaseCommandRetiredError extends Error {
  override readonly name = "CodebaseCommandRetiredError";

  constructor() {
    super("codebase_command_generation_retired");
  }
}

class CodebaseCommandGeneration {
  readonly #gateway: CodebaseGateway;
  readonly #settlers = new Set<() => void>();
  readonly #reindexTails = new Map<string, Promise<void>>();
  #retired = false;

  constructor(gateway: CodebaseGateway) {
    this.#gateway = gateway;
  }

  search(cwd: string | undefined, query: string, limit: number): Promise<CodebaseSearchHit[]> {
    return this.#execute(() => this.#gateway.search({ cwd, query, limit })).then(async (hits) => {
      await this.#repairStatus(cwd);
      this.#assertCurrent();
      return hits;
    });
  }

  reindex(cwd: string | undefined): Promise<CodebaseReindexOperation> {
    const identity = cwd ?? "";
    const result = this.#settle(this.#reindexTails.get(identity) ?? Promise.resolve()).then(
      async () => {
        const operation = await this.#execute(() => this.#gateway.reindex(cwd));
        commitCodebaseReindexStarted({ cwd }, operation.operationId);
        await this.#repairStatus(cwd);
        this.#assertCurrent();
        return operation;
      },
    );
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#reindexTails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#reindexTails.get(identity) === settlement) this.#reindexTails.delete(identity);
    });
    return result;
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    for (const settle of [...this.#settlers]) settle();
    this.#settlers.clear();
    this.#reindexTails.clear();
  }

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  async #repairStatus(cwd: string | undefined): Promise<void> {
    try {
      await this.#settle(
        queryClient.invalidateQueries({
          queryKey: [CODEBASE_STATUS_KEY, { cwd }],
          exact: true,
        }),
      );
    } catch (error) {
      if (this.#retired) throw error;
      // The exact command response remains authoritative. Runtime events and
      // the next status read retain the repair path.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    this.#assertCurrent();
    return new Promise<T>((resolve, reject) => {
      let pending = true;
      const finish = () => {
        if (!pending) return false;
        pending = false;
        this.#settlers.delete(retire);
        return true;
      };
      const retire = () => {
        if (finish()) reject(new CodebaseCommandRetiredError());
      };
      this.#settlers.add(retire);
      operation.then(
        (value) => {
          if (finish()) resolve(value);
        },
        (error: unknown) => {
          if (finish()) reject(error);
        },
      );
      if (this.#retired) retire();
    });
  }

  #assertCurrent(): void {
    if (this.#retired) throw new CodebaseCommandRetiredError();
  }
}

/** Owns every Codebase command and status projection for one exact Plugin Host
 * and Runtime generation. */
export class CodebaseCommandOwner {
  static #active: CodebaseCommandOwner | null = null;

  readonly #gateway: CodebaseGateway;
  #generation: CodebaseCommandGeneration;
  #disposed = false;

  private constructor(gateway: CodebaseGateway) {
    this.#gateway = gateway;
    this.#generation = new CodebaseCommandGeneration(gateway);
  }

  static install(gateway: CodebaseGateway): CodebaseCommandOwner {
    const predecessor = CodebaseCommandOwner.#active;
    const owner = new CodebaseCommandOwner(gateway);
    CodebaseCommandOwner.#active = owner;
    predecessor?.dispose();
    return owner;
  }

  static current(): CodebaseCommandOwner {
    const owner = CodebaseCommandOwner.#active;
    if (!owner || owner.#disposed) throw new Error("Codebase command owner is not installed");
    return owner;
  }

  search(cwd: string | undefined, query: string, limit = 12): Promise<CodebaseSearchHit[]> {
    return this.#generation.search(cwd, query, limit);
  }

  reindex(cwd: string | undefined): Promise<CodebaseReindexOperation> {
    return this.#generation.reindex(cwd);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || CodebaseCommandOwner.#active !== this) return;
    const predecessor = this.#generation;
    this.#generation = new CodebaseCommandGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (CodebaseCommandOwner.#active === this) CodebaseCommandOwner.#active = null;
  }
}

export function codebaseCommandWasRetired(error: unknown): boolean {
  return error instanceof CodebaseCommandRetiredError;
}
