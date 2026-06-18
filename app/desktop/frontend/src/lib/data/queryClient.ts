import { QueryClient } from "@tanstack/react-query";

// Single QueryClient for the app. Defaults are conservative: no auto-retry
// on failure (the user can manually refetch via re-render / staleness), and
// a 1-minute default stale window for resources we don't override.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 60_000,
    },
  },
});
