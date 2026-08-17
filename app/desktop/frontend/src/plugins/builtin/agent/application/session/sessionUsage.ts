import type { Query } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import type { AgentRuntimeGateway, AgentSessionUsage } from "../ports/runtimeGateway";

export const AGENT_SESSION_USAGE_KEY = "usage.session";

class AgentSessionUsageOwnerRetiredError extends Error {
  override readonly name = "AgentSessionUsageOwnerRetiredError";

  constructor() {
    super("agent_session_usage_owner_retired");
  }
}

/** Exact Agent Runtime gateway generation allowed to populate Session usage cache. */
export class AgentSessionUsageOwner {
  static #active: AgentSessionUsageOwner | null = null;

  readonly #lifetime = new AbortController();
  #retired = false;

  private constructor(private readonly gateway: AgentRuntimeGateway) {}

  static install(gateway: AgentRuntimeGateway): AgentSessionUsageOwner {
    const predecessor = AgentSessionUsageOwner.#active;
    const owner = new AgentSessionUsageOwner(gateway);
    AgentSessionUsageOwner.#active = owner;
    if (predecessor) predecessor.#retire();
    owner.#replaceCachedWriter();
    return owner;
  }

  static current(): AgentSessionUsageOwner {
    const owner = AgentSessionUsageOwner.#active;
    if (!owner) throw new AgentSessionUsageOwnerRetiredError();
    return owner;
  }

  async load(sessionId: string, querySignal: AbortSignal): Promise<AgentSessionUsage> {
    this.#assertCurrent();
    const attempt = new AbortController();
    const abortFromQuery = () => attempt.abort(querySignal.reason);
    const abortFromLifetime = () => attempt.abort(this.#lifetime.signal.reason);
    if (querySignal.aborted) abortFromQuery();
    else querySignal.addEventListener("abort", abortFromQuery, { once: true });
    if (this.#lifetime.signal.aborted) abortFromLifetime();
    else this.#lifetime.signal.addEventListener("abort", abortFromLifetime, { once: true });
    try {
      const usage = await this.gateway.loadSessionUsage(sessionId, attempt.signal);
      this.#assertCurrent();
      return usage;
    } finally {
      querySignal.removeEventListener("abort", abortFromQuery);
      this.#lifetime.signal.removeEventListener("abort", abortFromLifetime);
    }
  }

  dispose(): void {
    if (this.#retired) return;
    const current = AgentSessionUsageOwner.#active === this;
    const cachedWriters = current ? this.#cachedWriters() : undefined;
    if (current) AgentSessionUsageOwner.#active = null;
    this.#retire();
    if (cachedWriters && cachedWriters.size > 0) {
      void queryClient.cancelQueries({ predicate: (query) => cachedWriters.has(query) });
    }
  }

  #replaceCachedWriter(): void {
    const cachedWriters = this.#cachedWriters();
    if (cachedWriters.size === 0) return;
    const ownedWriter = (query: Query) => cachedWriters.has(query);
    void queryClient.cancelQueries({ predicate: ownedWriter }).then(() => {
      if (this.#retired || AgentSessionUsageOwner.#active !== this) return;
      void queryClient.resetQueries({ predicate: ownedWriter });
    });
  }

  #cachedWriters(): ReadonlySet<Query> {
    return new Set(queryClient.getQueryCache().findAll({ queryKey: [AGENT_SESSION_USAGE_KEY] }));
  }

  #assertCurrent(): void {
    if (this.#retired || AgentSessionUsageOwner.#active !== this) {
      throw new AgentSessionUsageOwnerRetiredError();
    }
  }

  #retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#lifetime.abort(new AgentSessionUsageOwnerRetiredError());
  }
}

export function useAgentSessionUsage(sessionId: string | undefined) {
  return useQuery({
    queryKey: [AGENT_SESSION_USAGE_KEY, sessionId],
    queryFn: ({ signal }) => AgentSessionUsageOwner.current().load(sessionId ?? "", signal),
    enabled: Boolean(sessionId),
  });
}
