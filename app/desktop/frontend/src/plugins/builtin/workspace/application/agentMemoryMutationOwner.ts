import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type {
  AgentMemoryAddInput,
  AgentMemoryDecision,
  AgentMemoryGateway,
} from "./ports/agentMemoryGateway";
import {
  WORKSPACE_AGENT_MEMORY_KEY,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "./workspaceQueries";

class AgentMemoryMutationRetiredError extends Error {
  override readonly name = "AgentMemoryMutationRetiredError";

  constructor() {
    super("agent_memory_mutation_generation_retired");
  }
}

interface AgentMemoryMutation<T> {
  execute(): Promise<T>;
  commit?(result: T): void;
}

class AgentMemoryMutationGeneration {
  readonly #gateway: AgentMemoryGateway;
  readonly #retiredError = new AgentMemoryMutationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #tails = new Map<string, Promise<void>>();

  constructor(gateway: AgentMemoryGateway) {
    this.#gateway = gateway;
  }

  review(id: string, decision: AgentMemoryDecision): Promise<void> {
    return this.#run(id, { execute: () => this.#gateway.review(id, decision) });
  }

  updateContent(id: string, content: string): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.updateContent(id, content),
      commit: commitAgentMemoryItem,
    }).then(() => undefined);
  }

  setPinned(id: string, pinned: boolean): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.setPinned(id, pinned),
      commit: commitAgentMemoryItem,
    }).then(() => undefined);
  }

  delete(id: string): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.delete(id),
      commit: () => removeAgentMemoryItem(id),
    });
  }

  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
    return this.#run(`add\u0000${input.scope}\u0000${input.cwd ?? ""}\u0000${input.content}`, {
      execute: () => this.#gateway.add(input),
      commit: (saved) => commitAddedAgentMemory(input, saved),
    });
  }

  retire(): void {
    this.#cohort.retire();
    this.#tails.clear();
  }

  #run<T>(identity: string, mutation: AgentMemoryMutation<T>): Promise<T> {
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(async () => {
      this.#assertCurrent();
      const value = await this.#settle(mutation.execute());
      this.#assertCurrent();
      mutation.commit?.(value);
      await this.#repairProjection();
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

  async #repairProjection(): Promise<void> {
    try {
      await this.#settle(queryClient.invalidateQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] }));
    } catch (error) {
      if (error === this.#retiredError) throw error;
      // The accepted mutation response is authoritative. A failed cache repair
      // is not a command failure; workspace events and the next read retry it.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Own every Agent Memory mutation and cache projection for one exact Plugin
 * Host / Runtime generation. Successor-first replacement retires both queued
 * admissions and non-cooperative in-flight settlements without draining them
 * through a newly installed gateway. */
export class AgentMemoryMutationOwner {
  readonly #gateway: AgentMemoryGateway;
  #generation: AgentMemoryMutationGeneration;
  #disposed = false;

  private constructor(gateway: AgentMemoryGateway) {
    this.#gateway = gateway;
    this.#generation = new AgentMemoryMutationGeneration(gateway);
  }

  static install(gateway: AgentMemoryGateway): AgentMemoryMutationOwner {
    const owner = new AgentMemoryMutationOwner(gateway);
    agentMemoryMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): AgentMemoryMutationOwner {
    const owner = agentMemoryMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Agent memory mutation owner is not installed");
    return owner;
  }

  review(id: string, decision: AgentMemoryDecision): Promise<void> {
    return this.#generation.review(id, decision);
  }

  updateContent(id: string, content: string): Promise<void> {
    return this.#generation.updateContent(id, content);
  }

  setPinned(id: string, pinned: boolean): Promise<void> {
    return this.#generation.setPinned(id, pinned);
  }

  delete(id: string): Promise<void> {
    return this.#generation.delete(id);
  }

  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
    return this.#generation.add(input);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !agentMemoryMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new AgentMemoryMutationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    agentMemoryMutationPublication.withdraw(this);
  }
}

const agentMemoryMutationPublication = createPublicationSlot<AgentMemoryMutationOwner>();

export function agentMemoryMutationWasRetired(error: unknown): boolean {
  return error instanceof AgentMemoryMutationRetiredError;
}

export function agentMemoryQuery(scope: AgentMemoryQuery["scope"], cwd?: string): AgentMemoryQuery {
  return scope === "user" ? { scope } : { scope, cwd };
}

function commitAgentMemoryItem(saved: AgentMemoryEntry): void {
  queryClient.setQueriesData<AgentMemoryEntry[]>(
    { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
    (current) => {
      if (!current) return current;
      const index = current.findIndex((item) => item.id === saved.id);
      if (index < 0) return current;
      return current.map((item) => (item.id === saved.id ? saved : item));
    },
  );
}

function commitAddedAgentMemory(input: AgentMemoryAddInput, saved: AgentMemoryEntry): void {
  const query = agentMemoryQuery(input.scope, input.cwd);
  queryClient.setQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query], (current) =>
    current ? [saved, ...current] : current,
  );
}

function removeAgentMemoryItem(id: string): void {
  queryClient.setQueriesData<AgentMemoryEntry[]>(
    { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
    (current) => current?.filter((item) => item.id !== id),
  );
}
