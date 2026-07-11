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

/** Build a cached read hook whose parameters are part of the cache identity. */
export function createParameterizedDataQuery<P, T>(
  key: string,
): (params: P | undefined) => UseQueryResult<T> {
  return (params) =>
    useQuery({
      queryKey: [key, params],
      queryFn: resolve<T, P>(key, params),
      enabled: params !== undefined,
      placeholderData: keepPreviousData,
      ...STATIC_QUERY_OPTIONS,
    });
}
