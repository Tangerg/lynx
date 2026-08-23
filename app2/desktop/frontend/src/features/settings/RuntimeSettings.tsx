import { useCallback, useEffect, useRef, useState } from "react";

import {
  connectRemoteRuntime,
  forgetRemoteRuntime,
  remoteRuntimeState,
  useLocalRuntime,
  useRemoteRuntime,
  type RemoteRuntimeState,
} from "../../runtime/desktopBridge";
import { useLocalization, type Translate } from "../localization/Localization";

export function RuntimeSettings({
  onRuntimeChanged,
}: {
  onRuntimeChanged(): Promise<void>;
}) {
  const { t } = useLocalization();
  const [state, setState] = useState<RemoteRuntimeState>();
  const [endpoint, setEndpoint] = useState("");
  const [token, setToken] = useState("");
  const [pending, setPending] = useState(false);
  const [confirmForget, setConfirmForget] = useState(false);
  const [error, setError] = useState<string>();
  const mounted = useRef(true);
  const operationInFlight = useRef(false);

  const load = useCallback(async () => {
    setError(undefined);
    try {
      const value = await remoteRuntimeState();
      if (!mounted.current) return;
      setState(value);
      setEndpoint(value.endpoint ?? "");
    } catch (failure) {
      if (mounted.current) setError(messageOf(failure, t));
    }
  }, [t]);

  useEffect(() => {
    let current = true;
    void remoteRuntimeState()
      .then((value) => {
        if (!current) return;
        setState(value);
        setEndpoint(value.endpoint ?? "");
      })
      .catch((failure) => {
        if (current) setError(messageOf(failure, t));
      });
    return () => {
      current = false;
    };
  }, [t]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const mutate = async (
    operation: () => Promise<RemoteRuntimeState>,
    reconnect: boolean,
  ) => {
    if (operationInFlight.current) return;
    operationInFlight.current = true;
    setPending(true);
    setError(undefined);
    try {
      const next = await operation();
      if (mounted.current) {
        setState(next);
        setEndpoint(next.endpoint ?? "");
        setConfirmForget(false);
      }
      if (reconnect) await onRuntimeChanged();
    } catch (failure) {
      if (mounted.current) setError(messageOf(failure, t));
    } finally {
      operationInFlight.current = false;
      if (mounted.current) setPending(false);
    }
  };

  if (state === undefined) {
    return (
      <div className="settings-state" aria-busy={error === undefined}>
        {error ? (
          <>
            <p role="alert">{error}</p>
            <button type="button" onClick={() => void load()}>
              {t("settings.runtime.tryAgain")}
            </button>
          </>
        ) : (
          <p>{t("settings.runtime.loading")}</p>
        )}
      </div>
    );
  }

  const connectedLabel = state.active
    ? state.connected
      ? t("settings.runtime.status.remoteActive")
      : t("settings.runtime.status.remoteUnavailable")
    : t("settings.runtime.status.localActive");

  return (
    <div className="runtime-settings">
      <section
        className="settings-section"
        aria-labelledby="runtime-active-title"
      >
        <header>
          <div>
            <h2 id="runtime-active-title">{t("settings.runtime.active")}</h2>
            <p>{t("settings.runtime.activeDetail")}</p>
          </div>
          <span
            className="runtime-state-chip"
            data-connected={!state.active || state.connected}
          >
            {connectedLabel}
          </span>
        </header>
        <div className="runtime-connection-card">
          <dl>
            <div>
              <dt>{t("settings.runtime.target")}</dt>
              <dd>
                {state.active
                  ? state.serverName || t("settings.runtime.remote")
                  : t("settings.runtime.local")}
              </dd>
            </div>
            <div>
              <dt>{t("settings.runtime.endpoint")}</dt>
              <dd>{state.active ? state.endpoint : t("settings.runtime.privateLoopback")}</dd>
            </div>
          </dl>
          <div className="runtime-connection-actions">
            {state.active ? (
              <button
                type="button"
                disabled={pending}
                onClick={() => void mutate(useLocalRuntime, true)}
              >
                {t("settings.runtime.useLocal")}
              </button>
            ) : state.configured ? (
              <button
                type="button"
                disabled={pending}
                onClick={() => void mutate(useRemoteRuntime, true)}
              >
                {t("settings.runtime.useSavedRemote")}
              </button>
            ) : null}
          </div>
          {state.detail ? (
            <p className="settings-inline-error">{state.detail}</p>
          ) : null}
        </div>
      </section>

      <section className="settings-section" aria-labelledby="runtime-remote-title">
        <header>
          <div>
            <h2 id="runtime-remote-title">{t("settings.runtime.remote")}</h2>
            <p>{t("settings.runtime.remoteDetail")}</p>
          </div>
        </header>
        <form
          className="remote-runtime-form"
          onSubmit={(event) => {
            event.preventDefault();
            void mutate(
              () => connectRemoteRuntime(endpoint.trim(), token),
              true,
            );
          }}
        >
          <label>
            <span>{t("settings.runtime.httpsOrigin")}</span>
            <input
              type="url"
              inputMode="url"
              autoComplete="url"
              required
              placeholder="https://runtime.example.com"
              value={endpoint}
              onChange={(event) => setEndpoint(event.target.value)}
            />
          </label>
          <label>
            <span>{t("settings.runtime.bearerSecret")}</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              minLength={16}
              placeholder={
                state.configured
                  ? t("settings.runtime.replacementSecret")
                  : t("settings.runtime.keyringSecret")
              }
              value={token}
              onChange={(event) => setToken(event.target.value)}
            />
          </label>
          <footer>
            <span>{t("settings.runtime.secretNote")}</span>
            <button
              type="submit"
              disabled={pending || endpoint.trim() === "" || token.length < 16}
            >
              {pending
                ? t("settings.runtime.connecting")
                : state.configured
                  ? t("settings.runtime.replace")
                  : t("settings.runtime.connect")}
            </button>
          </footer>
        </form>
      </section>

      {state.configured ? (
        <section
          className="settings-section runtime-danger"
          aria-labelledby="runtime-forget-title"
        >
          <header>
            <div>
              <h2 id="runtime-forget-title">{t("settings.runtime.forgetTitle")}</h2>
              <p>{t("settings.runtime.forgetDetail")}</p>
            </div>
          </header>
          <div>
            {confirmForget ? (
              <>
                <span>{t("settings.runtime.irreversible")}</span>
                <button
                  type="button"
                  disabled={pending}
                  onClick={() => setConfirmForget(false)}
                >
                  {t("settings.runtime.keep")}
                </button>
                <button
                  className="danger-action"
                  type="button"
                  disabled={pending}
                  onClick={() => void mutate(forgetRemoteRuntime, state.active)}
                >
                  {t("settings.runtime.forgetRemote")}
                </button>
              </>
            ) : (
              <button
                type="button"
                disabled={pending}
                onClick={() => setConfirmForget(true)}
              >
                {t("settings.runtime.forget")}
              </button>
            )}
          </div>
        </section>
      ) : null}
      {error ? (
        <p className="settings-inline-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function messageOf(error: unknown, t: Translate) {
  return error instanceof Error ? error.message : t("settings.runtime.failed");
}
