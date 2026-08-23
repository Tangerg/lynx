import { useQuery } from "@tanstack/react-query";
import type {
  DiscoverResponse,
  RuntimeConnection,
} from "@lyra/runtime-contract";

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

function WorkspaceShell(props: {
  connection: RuntimeConnection;
  discovery: DiscoverResponse;
}) {
  const enabledFeatures = Object.values(
    props.discovery.capabilities.features,
  ).filter((feature) => feature.enabled).length;
  return (
    <main className="app-shell">
      <aside className="work-index" aria-labelledby="work-index-title">
        <header className="panel-header window-drag">
          <span className="eyebrow">Lyra</span>
          <h1 id="work-index-title">Work Index</h1>
        </header>
        <section className="workspace-card" aria-label="Current workspace">
          <span className="status-dot" aria-hidden="true" />
          <div>
            <strong>Current workspace</strong>
            <p title={props.discovery.serverInfo.defaultWorkspace.path}>
              {compactPath(props.discovery.serverInfo.defaultWorkspace.path)}
            </p>
          </div>
        </section>
      </aside>

      <section className="narrative" aria-labelledby="narrative-title">
        <header className="narrative-header window-drag">
          <div>
            <span className="eyebrow">Agent Narrative</span>
            <h2 id="narrative-title">Runtime connected</h2>
          </div>
          <span className="online-pill">
            <span aria-hidden="true" />
            Online
          </span>
        </header>
        <div className="narrative-empty">
          <div className="orb" aria-hidden="true">
            <span />
          </div>
          <h3>A clean foundation is ready.</h3>
          <p>
            The Desktop is talking directly to the supervised Lyra Runtime
            through the generated protocol client. Sessions and Runs will enter
            here as their vertical slices migrate.
          </p>
        </div>
      </section>

      <aside className="context-dock" aria-labelledby="context-title">
        <header className="panel-header window-drag">
          <span className="eyebrow">Context Dock</span>
          <h2 id="context-title">Runtime</h2>
        </header>
        <dl className="runtime-facts">
          <Fact
            label="Generation"
            value={String(props.connection.generation)}
            numeric
          />
          <Fact label="Protocol" value={props.discovery.protocolVersion} />
          <Fact
            label="Features online"
            value={String(enabledFeatures)}
            numeric
          />
          <Fact
            label="Streaming methods"
            value={String(props.discovery.capabilities.streamingMethods.length)}
            numeric
          />
        </dl>
        <div className="identity-card">
          <span>Instance</span>
          <code>{shortIdentity(props.connection.instanceId)}</code>
        </div>
      </aside>
    </main>
  );
}

function Fact(props: { label: string; value: string; numeric?: boolean }) {
  return (
    <div>
      <dt>{props.label}</dt>
      <dd className={props.numeric ? "tabular" : undefined}>{props.value}</dd>
    </div>
  );
}

function compactPath(path: string): string {
  const parts = path.split("/").filter(Boolean);
  return parts.length <= 2 ? path : `…/${parts.slice(-2).join("/")}`;
}

function shortIdentity(identity: string): string {
  return identity.length <= 18
    ? identity
    : `${identity.slice(0, 10)}…${identity.slice(-6)}`;
}

function messageOf(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unknown error interrupted startup.";
}
