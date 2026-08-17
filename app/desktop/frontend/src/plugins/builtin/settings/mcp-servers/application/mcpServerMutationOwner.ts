import { queryClient } from "@/lib/queryClient";
import type { MCPServerInput } from "./mcpServerInput";
import { MCP_SERVERS_KEY, MCP_TOOLS_KEY, type MCPServerSettings } from "./mcpServerQueries";
import type { MCPServerGateway, MCPServerTestOutcome } from "./ports/mcpServerGateway";

const AUTHORIZATION_ATTEMPT_POLL_MS = 500;

class MCPServerMutationRetiredError extends Error {
  override readonly name = "MCPServerMutationRetiredError";

  constructor() {
    super("mcp_server_mutation_generation_retired");
  }
}

interface MCPServerMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
}

class MCPServerMutationGeneration {
  readonly #gateway: MCPServerGateway;
  readonly #lifetime = new AbortController();
  readonly #retirement: Promise<void>;
  readonly #retiredError = new MCPServerMutationRetiredError();
  #signalRetirement!: () => void;
  readonly #tails = new Map<string, Promise<void>>();
  #retired = false;

  constructor(gateway: MCPServerGateway) {
    this.#gateway = gateway;
    this.#retirement = new Promise<void>((resolve) => {
      this.#signalRetirement = resolve;
    });
  }

  create(input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#run(input.name, {
      execute: () => this.#gateway.create(input),
      commit: commitMCPServerSaved,
    });
  }

  update(name: string, input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#run(name, {
      execute: () => this.#gateway.update(name, input),
      commit: commitMCPServerSaved,
    });
  }

  setEnabled(name: string, enabled: boolean): Promise<MCPServerSettings> {
    return this.#run(name, {
      execute: () => this.#gateway.setEnabled(name, enabled),
      commit: commitMCPServerSaved,
    });
  }

  delete(name: string): Promise<void> {
    return this.#run(name, {
      execute: () => this.#gateway.delete(name),
      commit: () => removeMCPServer(name),
    });
  }

  test(input: MCPServerInput): Promise<MCPServerTestOutcome> {
    return this.#execute(() => this.#gateway.test(input));
  }

  async authorize(name: string, callerSignal?: AbortSignal): Promise<void> {
    const signal = callerSignal
      ? AbortSignal.any([callerSignal, this.#lifetime.signal])
      : this.#lifetime.signal;
    let attempt = await this.#execute(() => this.#gateway.createAuthorizationAttempt(name, signal));
    while (attempt.status === "pending") {
      await this.#settle(authorizationPollDelay(signal));
      attempt = await this.#execute(() =>
        this.#gateway.getAuthorizationAttempt(attempt.id, signal),
      );
    }
    await this.#repairProjection();
    this.#assertCurrent();
    if (attempt.status === "failed") throw new Error(attempt.error);
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#lifetime.abort(this.#retiredError);
    this.#signalRetirement();
  }

  #run<T>(identity: string, mutation: MCPServerMutation<T>): Promise<T> {
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(async () => {
      const value = await this.#execute(mutation.execute);
      mutation.commit(value);
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

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  async #repairProjection(): Promise<void> {
    try {
      await Promise.all([
        this.#settle(queryClient.invalidateQueries({ queryKey: [MCP_SERVERS_KEY] })),
        this.#settle(queryClient.invalidateQueries({ queryKey: [MCP_TOOLS_KEY] })),
      ]);
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

/** Owns MCP server commands, authorization polling, and cache projection for
 * one exact Plugin Host and Runtime generation. */
export class MCPServerMutationOwner {
  static #active: MCPServerMutationOwner | null = null;

  readonly #gateway: MCPServerGateway;
  #generation: MCPServerMutationGeneration;
  #disposed = false;

  private constructor(gateway: MCPServerGateway) {
    this.#gateway = gateway;
    this.#generation = new MCPServerMutationGeneration(gateway);
  }

  static install(gateway: MCPServerGateway): MCPServerMutationOwner {
    const predecessor = MCPServerMutationOwner.#active;
    const owner = new MCPServerMutationOwner(gateway);
    MCPServerMutationOwner.#active = owner;
    predecessor?.dispose();
    return owner;
  }

  static current(): MCPServerMutationOwner {
    const owner = MCPServerMutationOwner.#active;
    if (!owner || owner.#disposed) throw new Error("MCP server mutation owner is not installed");
    return owner;
  }

  create(input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#generation.create(input);
  }

  update(name: string, input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#generation.update(name, input);
  }

  setEnabled(name: string, enabled: boolean): Promise<MCPServerSettings> {
    return this.#generation.setEnabled(name, enabled);
  }

  delete(name: string): Promise<void> {
    return this.#generation.delete(name);
  }

  test(input: MCPServerInput): Promise<MCPServerTestOutcome> {
    return this.#generation.test(input);
  }

  authorize(name: string, signal?: AbortSignal): Promise<void> {
    return this.#generation.authorize(name, signal);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || MCPServerMutationOwner.#active !== this) return;
    const predecessor = this.#generation;
    this.#generation = new MCPServerMutationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (MCPServerMutationOwner.#active === this) MCPServerMutationOwner.#active = null;
  }
}

export function mcpServerMutationWasRetired(error: unknown): boolean {
  return error instanceof MCPServerMutationRetiredError;
}

function commitMCPServerSaved(saved: MCPServerSettings): void {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], (current) => {
    if (!current) return current;
    const index = current.findIndex((server) => server.id === saved.id);
    if (index < 0) return [...current, saved];
    return current.map((server) => (server.id === saved.id ? saved : server));
  });
}

function removeMCPServer(name: string): void {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], (current) =>
    current?.filter((server) => server.id !== name),
  );
}

function authorizationPollDelay(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(signal.reason);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(done, AUTHORIZATION_ATTEMPT_POLL_MS);
    function done(): void {
      signal.removeEventListener("abort", aborted);
      resolve();
    }
    function aborted(): void {
      clearTimeout(timer);
      reject(signal.reason);
    }
    signal.addEventListener("abort", aborted, { once: true });
  });
}
