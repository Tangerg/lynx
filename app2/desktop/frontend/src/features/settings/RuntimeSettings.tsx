import { useCallback, useEffect, useRef, useState } from "react";

import {
  connectRemoteRuntime,
  forgetRemoteRuntime,
  remoteRuntimeState,
  useLocalRuntime,
  useRemoteRuntime,
  type RemoteRuntimeState,
} from "../../runtime/desktopBridge";

export function RuntimeSettings({
  onRuntimeChanged,
}: {
  onRuntimeChanged(): Promise<void>;
}) {
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
      if (mounted.current) setError(messageOf(failure));
    }
  }, []);

  useEffect(() => {
    let current = true;
    void remoteRuntimeState()
      .then((value) => {
        if (!current) return;
        setState(value);
        setEndpoint(value.endpoint ?? "");
      })
      .catch((failure) => {
        if (current) setError(messageOf(failure));
      });
    return () => {
      current = false;
    };
  }, []);

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
      if (mounted.current) setError(messageOf(failure));
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
              Try again
            </button>
          </>
        ) : (
          <p>Loading Runtime connection…</p>
        )}
      </div>
    );
  }

  const connectedLabel = state.active
    ? state.connected
      ? "Remote active"
      : "Remote unavailable"
    : "Local active";

  return (
    <div className="runtime-settings">
      <section
        className="settings-section"
        aria-labelledby="runtime-active-title"
      >
        <header>
          <div>
            <h2 id="runtime-active-title">Active Runtime</h2>
            <p>
              Desktop changes deployment targets without changing Lyra Protocol.
            </p>
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
              <dt>Target</dt>
              <dd>
                {state.active
                  ? state.serverName || "Remote Runtime"
                  : "Local Runtime"}
              </dd>
            </div>
            <div>
              <dt>Endpoint</dt>
              <dd>{state.active ? state.endpoint : "Private loopback"}</dd>
            </div>
          </dl>
          <div className="runtime-connection-actions">
            {state.active ? (
              <button
                type="button"
                disabled={pending}
                onClick={() => void mutate(useLocalRuntime, true)}
              >
                Use local Runtime
              </button>
            ) : state.configured ? (
              <button
                type="button"
                disabled={pending}
                onClick={() => void mutate(useRemoteRuntime, true)}
              >
                Use saved remote
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
            <h2 id="runtime-remote-title">Remote Runtime</h2>
            <p>
              HTTPS origin and bearer secret are verified before the profile
              becomes active.
            </p>
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
            <span>HTTPS origin</span>
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
            <span>Bearer secret</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              minLength={16}
              placeholder={
                state.configured
                  ? "Enter a replacement secret"
                  : "Stored in the system keyring"
              }
              value={token}
              onChange={(event) => setToken(event.target.value)}
            />
          </label>
          <footer>
            <span>Secrets never enter the persisted profile.</span>
            <button
              type="submit"
              disabled={pending || endpoint.trim() === "" || token.length < 16}
            >
              {pending
                ? "Connecting…"
                : state.configured
                  ? "Replace connection"
                  : "Connect remote"}
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
              <h2 id="runtime-forget-title">Forget remote profile</h2>
              <p>
                Remove the saved endpoint identity and bearer secret from this
                device.
              </p>
            </div>
          </header>
          <div>
            {confirmForget ? (
              <>
                <span>This cannot be undone.</span>
                <button
                  type="button"
                  disabled={pending}
                  onClick={() => setConfirmForget(false)}
                >
                  Keep profile
                </button>
                <button
                  className="danger-action"
                  type="button"
                  disabled={pending}
                  onClick={() => void mutate(forgetRemoteRuntime, state.active)}
                >
                  Forget remote
                </button>
              </>
            ) : (
              <button
                type="button"
                disabled={pending}
                onClick={() => setConfirmForget(true)}
              >
                Forget…
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

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Runtime connection failed.";
}
