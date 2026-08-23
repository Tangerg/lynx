import { useQuery } from "@tanstack/react-query";
import type { RuntimeConnection } from "@lyra/runtime-contract";
import { useState } from "react";

import { useLocalization } from "./features/localization/Localization";
import { presentRuntimeError } from "./features/localization/presentRuntimeError";
import { WorkspaceShell } from "./features/workspace/WorkspaceShell";
import { loadDesktopBootstrap, useLocalRuntime } from "./runtime/desktopBridge";
import { discoverRuntime, runtimeQueryKeys } from "./runtime/runtimeQueries";
import "./styles/index.css";

export function App() {
  const { t } = useLocalization();
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
        title={t("app.runtimeUnavailable")}
        detail={presentRuntimeError(
          bootstrap.error,
          t("app.unknownStartupError"),
          t,
        )}
        retry={bootstrap.refetch}
        recovery={{
          label: t("app.useLocalRuntime"),
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
        title={t("app.runtimeHandshakeFailed")}
        detail={presentRuntimeError(
          discovery.error,
          t("app.unknownStartupError"),
          t,
        )}
        retry={discovery.refetch}
        recovery={{
          label: t("app.useLocalRuntime"),
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
  const { t } = useLocalization();
  return (
    <main
      className="boot-state"
      aria-busy="true"
      aria-label={t("app.connectingLabel")}
    >
      <div className="brand-mark" aria-hidden="true">
        L
      </div>
      <p>{t("app.connecting")}</p>
    </main>
  );
}

function Failure(props: {
  title: string;
  detail: string;
  retry: () => unknown;
  recovery?: { label: string; run(): Promise<void> };
}) {
  const { t } = useLocalization();
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
          {t("app.tryAgain")}
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
                .catch((error) =>
                  setRecoveryError(
                    presentRuntimeError(error, t("app.unknownStartupError"), t),
                  ),
                )
                .finally(() => setRecoveryPending(false));
            }}
          >
            {recoveryPending ? t("app.switchingRuntime") : props.recovery.label}
          </button>
        ) : null}
      </div>
      {recoveryError ? <p role="alert">{recoveryError}</p> : null}
    </main>
  );
}
