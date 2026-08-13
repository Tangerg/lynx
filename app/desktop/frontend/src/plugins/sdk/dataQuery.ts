// Cached read hooks over the DATA_PROVIDER extension point — the read half of
// the contract whose write half is `contributeDataProvider`. It sat in `lib/`
// and imported the SDK from there, which let a utility module depend on the
// plugin registry; the hooks ARE the registry's read surface, so they live with
// it.

import type { UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { lookupDataProvider } from "./selectors/runtime";

const STATIC_QUERY_OPTIONS = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

function resolve<T, P = void>(
  key: string,
  params?: P,
): (context: { signal: AbortSignal }) => Promise<T> {
  return ({ signal }) => {
    const fetcher = lookupDataProvider<T, P>(key);
    if (!fetcher) {
      return Promise.reject(new Error(`No data provider registered for key "${key}"`));
    }
    return fetcher(params, signal);
  };
}

/** Build a cached read hook for a parameterless data-provider contract. */
export function createDataQuery<T>(key: string): () => UseQueryResult<T> {
  return () => useQuery({ queryKey: [key], queryFn: resolve<T>(key), ...STATIC_QUERY_OPTIONS });
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
      queryFn: resolve<T, P>(key, params),
      enabled: params !== undefined,
      // Parameters are resource identity, not presentation state. Reusing the
      // prior key's value can display and mutate one session/workspace while
      // the UI already names another. A surface whose parameter variants are
      // genuinely interchangeable can opt into that behavior in its own hook.
      ...STATIC_QUERY_OPTIONS,
      refetchInterval: interval ? (query) => interval(query.state.data) : undefined,
    });
}
