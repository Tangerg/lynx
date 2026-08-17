// Cached read hooks over the DATA_PROVIDER extension point — the read half of
// the contract whose write half is `contributeDataProvider`. It sat in `lib/`
// and imported the SDK from there, which let a utility module depend on the
// plugin registry; the hooks ARE the registry's read surface, so they live with
// it.

import type { Query, UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import type { Host } from "dougong";
import { queryClient } from "@/lib/queryClient";
import type { Contribution } from "./contracts";
import { DATA_PROVIDER } from "./kernelPoints";
import { contributionsTo, publishedKernel, subscribeContributions } from "./kernel";
import type { DataProviderSpec } from "./types";

const STATIC_QUERY_OPTIONS = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

class DataProviderGenerationRetiredError extends Error {
  override readonly name = "DataProviderGenerationRetiredError";

  constructor() {
    super("data_provider_generation_retired");
  }
}

class DataProviderGeneration {
  readonly keys: ReadonlySet<string>;
  readonly #providers: ReadonlyMap<string, DataProviderSpec>;
  readonly #lifetime = new AbortController();
  #retired = false;

  constructor(
    readonly owner: Host | undefined,
    readonly entries: ReadonlyArray<Contribution<DataProviderSpec>>,
  ) {
    this.#providers = new Map(entries.map(({ key, item }) => [key, item]));
    this.keys = new Set(this.#providers.keys());
  }

  replacedKeysBy(successor: DataProviderGeneration): ReadonlySet<string> {
    const candidates = new Set([...this.keys, ...successor.keys]);
    if (this.owner !== successor.owner) return candidates;
    return new Set(
      [...candidates].filter((key) => this.#providers.get(key) !== successor.#providers.get(key)),
    );
  }

  async load<T, P>(key: string, params: P | undefined, querySignal: AbortSignal): Promise<T> {
    this.#assertCurrent();
    const provider = this.#providers.get(key);
    if (!provider) throw new Error(`No data provider registered for key "${key}"`);
    const attempt = new AbortController();
    const abortFromQuery = () => attempt.abort(querySignal.reason);
    const abortFromLifetime = () => attempt.abort(this.#lifetime.signal.reason);
    if (querySignal.aborted) abortFromQuery();
    else querySignal.addEventListener("abort", abortFromQuery, { once: true });
    if (this.#lifetime.signal.aborted) abortFromLifetime();
    else this.#lifetime.signal.addEventListener("abort", abortFromLifetime, { once: true });
    try {
      const value = await (provider.fetcher as (params?: P, signal?: AbortSignal) => Promise<T>)(
        params,
        attempt.signal,
      );
      this.#assertCurrent();
      return value;
    } finally {
      querySignal.removeEventListener("abort", abortFromQuery);
      this.#lifetime.signal.removeEventListener("abort", abortFromLifetime);
    }
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#lifetime.abort(new DataProviderGenerationRetiredError());
  }

  #assertCurrent(): void {
    if (this.#retired) throw new DataProviderGenerationRetiredError();
  }
}

class DataQueryOwner {
  #generation = new DataProviderGeneration(publishedKernel(), contributionsTo(DATA_PROVIDER));
  readonly #unsubscribe: () => void;
  #disposed = false;

  constructor() {
    this.#unsubscribe = subscribeContributions(() => this.#reconcileProviders());
  }

  load<T, P>(key: string, params: P | undefined, signal: AbortSignal): Promise<T> {
    return this.#generation.load<T, P>(key, params, signal);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#unsubscribe();
    this.#generation.retire();
  }

  #reconcileProviders(): void {
    if (this.#disposed) return;
    const entries = contributionsTo(DATA_PROVIDER);
    if (entries === this.#generation.entries) return;
    const predecessor = this.#generation;
    const successor = new DataProviderGeneration(publishedKernel(), entries);
    this.#generation = successor;
    predecessor.retire();
    this.#replaceCachedWriters(predecessor, successor);
  }

  #replaceCachedWriters(
    predecessor: DataProviderGeneration,
    successor: DataProviderGeneration,
  ): void {
    const affected = predecessor.replacedKeysBy(successor);
    if (affected.size === 0) return;
    const affectedQuery = (query: Query) => {
      const key = query.queryKey[0];
      return typeof key === "string" && affected.has(key);
    };
    const predecessorQueries = new Set(
      queryClient.getQueryCache().findAll({ predicate: affectedQuery }),
    );
    if (predecessorQueries.size === 0) return;
    const ownedQuery = (query: Query) => predecessorQueries.has(query);
    void queryClient.cancelQueries({ predicate: ownedQuery }).then(() => {
      if (this.#disposed || this.#generation !== successor) return;
      queryClient.removeQueries({
        predicate: (query) => {
          const key = query.queryKey[0];
          return ownedQuery(query) && typeof key === "string" && !successor.keys.has(key);
        },
      });
      void queryClient.resetQueries({
        predicate: (query) => {
          const key = query.queryKey[0];
          return ownedQuery(query) && typeof key === "string" && successor.keys.has(key);
        },
      });
    });
  }
}

const dataQueryOwner = new DataQueryOwner();

if (import.meta.hot) import.meta.hot.dispose(() => dataQueryOwner.dispose());

/** Build a cached read hook for a parameterless data-provider contract. */
export function createDataQuery<T>(key: string): () => UseQueryResult<T> {
  return () =>
    useQuery({
      queryKey: [key],
      queryFn: ({ signal }) => dataQueryOwner.load<T, void>(key, undefined, signal),
      ...STATIC_QUERY_OPTIONS,
    });
}

export interface ParameterizedQueryOptions<T> {
  /** Poll cadence derived from the latest data — return a ms interval to keep
   *  refetching, or false to stop. Use for server state with no push signal
   *  (e.g. an autonomous goal loop whose server-launched runs the client can't
   *  observe): poll only while it's live, idle otherwise. */
  refetchInterval?: (data: T | undefined) => number | false;
}

/** Build a cached read hook whose parameters are part of the cache identity. */
export function createParameterizedDataQuery<P, T>(
  key: string,
  options?: ParameterizedQueryOptions<T>,
): (params: P | undefined) => UseQueryResult<T> {
  const interval = options?.refetchInterval;
  return (params) =>
    useQuery({
      queryKey: [key, params],
      queryFn: ({ signal }) => dataQueryOwner.load<T, P>(key, params, signal),
      enabled: params !== undefined,
      // Parameters are resource identity, not presentation state. Reusing the
      // prior key's value can display and mutate one session/workspace while
      // the UI already names another. A surface whose parameter variants are
      // genuinely interchangeable can opt into that behavior in its own hook.
      ...STATIC_QUERY_OPTIONS,
      refetchInterval: interval ? (query) => interval(query.state.data) : undefined,
    });
}
