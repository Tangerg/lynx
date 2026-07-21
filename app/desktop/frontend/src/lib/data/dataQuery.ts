import type { UseQueryResult } from "@tanstack/react-query";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { lookupDataProvider } from "@/plugins/sdk";

const STATIC_QUERY_OPTIONS = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

function resolve<T, P = void>(key: string, params?: P): () => Promise<T> {
  return () => {
    const fetcher = lookupDataProvider<T, P>(key);
    if (!fetcher) {
      return Promise.reject(new Error(`No data provider registered for key "${key}"`));
    }
    return fetcher(params);
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
      placeholderData: keepPreviousData,
      ...STATIC_QUERY_OPTIONS,
      refetchInterval: interval ? (query) => interval(query.state.data) : undefined,
    });
}
