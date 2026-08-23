import { useQuery } from "@tanstack/react-query";
import type { RuntimeConnection } from "@lyra/runtime-contract";

import { WorkspaceShell } from "./features/workspace/WorkspaceShell";
import { loadDesktopBootstrap } from "./runtime/desktopBridge";
import { discoverRuntime } from "./runtime/runtimeQueries";
import "./styles.css";

export function App() {
  const bootstrap = useQuery({
    queryKey: ["desktop", "bootstrap"],
    queryFn: () => loadDesktopBootstrap(),
    retry: 3,
    retryDelay: (attempt) => Math.min(250 * 2 ** attempt, 2_000),
    refetchInterval: 2_000,
  });
  const connection = bootstrap.data?.runtime;
  const discovery = useQuery({
    queryKey: ["runtime", "discover", connection?.generation],
    queryFn: ({ signal }) =>
      discoverRuntime(connection as RuntimeConnection, signal),
    enabled: connection !== undefined,
    retry: 3,
    retryDelay: (attempt) => Math.min(250 * 2 ** attempt, 2_000),
  });

  if (bootstrap.isError) {
    return (
      <Failure
        title="Runtime unavailable"
        detail={messageOf(bootstrap.error)}
        retry={bootstrap.refetch}
      />
    );
  }
  if (!connection || discovery.isPending) {
    return <Loading />;
  }
  if (discovery.isError) {
    return (
      <Failure
        title="Runtime handshake failed"
        detail={messageOf(discovery.error)}
        retry={discovery.refetch}
      />
    );
  }
  return <WorkspaceShell connection={connection} discovery={discovery.data} />;
}

function Loading() {
  return (
    <main
      className="boot-state"
      aria-busy="true"
      aria-label="Connecting to Lyra Runtime"
    >
      <div className="brand-mark" aria-hidden="true">
        L
      </div>
      <p>Connecting to your Runtime…</p>
    </main>
  );
}

function Failure(props: {
  title: string;
  detail: string;
  retry: () => unknown;
}) {
  return (
    <main className="boot-state">
      <div className="brand-mark brand-mark-error" aria-hidden="true">
        !
      </div>
      <h1>{props.title}</h1>
      <p>{props.detail}</p>
      <button type="button" onClick={() => props.retry()}>
        Try again
      </button>
    </main>
  );
}

function messageOf(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unknown error interrupted startup.";
}
