import { useQuery } from "@tanstack/react-query";
import type { RuntimeConnection } from "@lyra/runtime-contract";
import { useState } from "react";

import { WorkspaceShell } from "./features/workspace/WorkspaceShell";
import {
  loadDesktopBootstrap,
  useLocalRuntime,
} from "./runtime/desktopBridge";
import { discoverRuntime, runtimeQueryKeys } from "./runtime/runtimeQueries";
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
    queryKey:
      connection === undefined
        ? ["runtime", "unselected", "discover"]
        : [...runtimeQueryKeys.scope(connection), "discover"],
    queryFn: ({ signal }) =>
      discoverRuntime(connection as RuntimeConnection, signal),
    enabled: connection !== undefined,
    retry: 3,
    retryDelay: (attempt) => Math.min(250 * 2 ** attempt, 2_000),
  });
  const refreshBootstrap = async () => {
    const result = await bootstrap.refetch();
    if (result.error) throw result.error;
  };
  const returnToLocal = async () => {
    await useLocalRuntime();
    await refreshBootstrap();
  };

  if (bootstrap.isError) {
    return (
      <Failure
        title="Runtime unavailable"
        detail={messageOf(bootstrap.error)}
        retry={bootstrap.refetch}
        recovery={{
          label: "Use local Runtime",
          run: returnToLocal,
        }}
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
        recovery={{
          label: "Use local Runtime",
          run: returnToLocal,
        }}
      />
    );
  }
  return (
    <WorkspaceShell
      key={`${connection.endpoint}:${connection.instanceId}:${connection.generation}`}
      connection={connection}
      discovery={discovery.data}
      onRuntimeChanged={refreshBootstrap}
    />
  );
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
  recovery?: { label: string; run(): Promise<void> };
}) {
  const [recoveryPending, setRecoveryPending] = useState(false);
  const [recoveryError, setRecoveryError] = useState<string>();
  return (
    <main className="boot-state">
      <div className="brand-mark brand-mark-error" aria-hidden="true">
        !
      </div>
      <h1>{props.title}</h1>
      <p>{props.detail}</p>
      <div className="boot-actions">
        <button type="button" onClick={() => props.retry()}>
          Try again
        </button>
        {props.recovery ? (
          <button
            type="button"
            disabled={recoveryPending}
            onClick={() => {
              const recovery = props.recovery;
              if (recovery === undefined) return;
              setRecoveryPending(true);
              setRecoveryError(undefined);
              void recovery
                .run()
                .catch((error) => setRecoveryError(messageOf(error)))
                .finally(() => setRecoveryPending(false));
            }}
          >
            {recoveryPending ? "Switching…" : props.recovery.label}
          </button>
        ) : null}
      </div>
      {recoveryError ? <p role="alert">{recoveryError}</p> : null}
    </main>
  );
}

function messageOf(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unknown error interrupted startup.";
}
