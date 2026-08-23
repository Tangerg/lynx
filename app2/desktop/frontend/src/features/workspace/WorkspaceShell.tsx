import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  DiscoverResponse,
  RuntimeConnection,
  Session,
} from "@lyra/runtime-contract";

import { GoalComposer } from "../goals/GoalComposer";
import { GoalTray } from "../goals/GoalTray";
import { useGoalActions } from "../goals/useGoalActions";
import { PlanCompact } from "../plan/PlanCompact";
import { NewSessionMenu } from "../sessions/NewSessionMenu";
import { SessionIndex } from "../sessions/SessionIndex";
import { compactPath } from "../sessions/sessionPresentation";
import { useSessionCatalog } from "../sessions/useSessionCatalog";
import {
  loadSessionSnapshot,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import {
  useRuntimeInvalidations,
  type RuntimeSyncState,
} from "../../runtime/useRuntimeInvalidations";

interface WorkspaceShellProps {
  connection: RuntimeConnection;
  discovery: DiscoverResponse;
}

export function WorkspaceShell(props: WorkspaceShellProps) {
  const connection = useMemo(
    () => props.connection,
    [
      props.connection.endpoint,
      props.connection.generation,
      props.connection.idempotencyNamespace,
      props.connection.instanceId,
      props.connection.localToken,
      props.connection.protocolVersion,
    ],
  );
  const [selectedSessionId, setSelectedSessionId] = useState<string>();
  const planEnabled = props.discovery.capabilities.features.plan?.enabled === true;
  const goalsEnabled = props.discovery.capabilities.features.goals?.enabled === true;
  const liveUpdatesEnabled =
    props.discovery.capabilities.streamingMethods.includes("runtime.subscribe") &&
    (
      [
        "sessions.changed",
        "runs.changed",
        "plan.changed",
        "goals.changed",
      ] as const
    ).every((topic) =>
      props.discovery.capabilities.runtimeTopics.includes(topic),
    );
  const syncState = useRuntimeInvalidations(connection, liveUpdatesEnabled);

  const catalog = useSessionCatalog(connection);
  const selectedSession = selectSession(catalog.sessions, selectedSessionId);
  const snapshot = useQuery({
    queryKey: runtimeQueryKeys.snapshot(
      connection,
      selectedSession?.id ?? "unselected",
    ),
    queryFn: ({ signal }) =>
      loadSessionSnapshot(connection, selectedSession?.id ?? "", signal),
    enabled: selectedSession !== undefined,
    retry: 2,
  });
  const goalActions = useGoalActions(connection, selectedSession?.id);
  const createSession = useCallback(
    async (request = {}) => {
      const session = await catalog.create(request);
      setSelectedSessionId(session.id);
      return session;
    },
    [catalog.create],
  );
  const removeSession = useCallback(
    async (session: Session) => {
      const fallback = catalog.sessions.find(
        (candidate) => candidate.id !== session.id,
      );
      await catalog.remove(session);
      if (selectedSession?.id === session.id) {
        setSelectedSessionId(fallback?.id);
      }
    },
    [catalog.remove, catalog.sessions, selectedSession?.id],
  );

  useEffect(() => {
    const createOnShortcut = (event: KeyboardEvent) => {
      if (
        event.key.toLocaleLowerCase() !== "n" ||
        (!event.metaKey && !event.ctrlKey) ||
        event.altKey ||
        event.shiftKey ||
        catalog.createPending
      ) {
        return;
      }
      event.preventDefault();
      void createSession().catch(() => undefined);
    };
    window.addEventListener("keydown", createOnShortcut);
    return () => window.removeEventListener("keydown", createOnShortcut);
  }, [catalog.createPending, createSession]);

  return (
    <main className="app-shell">
      <aside className="work-index" aria-labelledby="work-index-title">
        <header className="panel-header window-drag">
          <div>
            <span className="eyebrow">Lyra</span>
            <h1 id="work-index-title">Work Index</h1>
          </div>
          <NewSessionMenu
            pending={catalog.createPending}
            defaultWorkspace={props.discovery.serverInfo.defaultWorkspace.path}
            onCreate={createSession}
          />
        </header>
        <section className="workspace-card" aria-label="Runtime default workspace">
          <span className="status-dot" aria-hidden="true" />
          <div>
            <strong>Runtime default</strong>
            <p title={props.discovery.serverInfo.defaultWorkspace.path}>
              {compactPath(props.discovery.serverInfo.defaultWorkspace.path)}
            </p>
          </div>
        </section>
        <SessionIndex
          sessions={catalog.sessions}
          selectedId={selectedSession?.id}
          pending={catalog.query.isPending}
          error={catalog.query.error}
          actionPending={catalog.updatePending || catalog.removePending}
          hasMore={catalog.query.hasNextPage}
          loadingMore={catalog.query.isFetchingNextPage}
          onSelect={setSelectedSessionId}
          onUpdate={(session, patch) => catalog.update({ source: session, patch })}
          onRemove={removeSession}
          onRetry={() => void catalog.query.refetch()}
          onLoadMore={() => void catalog.query.fetchNextPage()}
        />
        {catalog.createError ? (
          <p className="sidebar-command-error" role="alert">
            {messageOf(catalog.createError)}
          </p>
        ) : null}
      </aside>

      <section className="narrative" aria-labelledby="narrative-title">
        <header className="narrative-header window-drag">
          <div className="narrative-heading">
            <span className="eyebrow">Agent Narrative</span>
            <h2 id="narrative-title">
              {selectedSession?.title || "Choose a session"}
            </h2>
          </div>
          <div className="narrative-tools window-no-drag">
            {selectedSession && planEnabled ? (
              <PlanCompact
                plan={snapshot.data?.plan}
                pending={snapshot.isPending}
                error={snapshot.isError}
              />
            ) : null}
            <ConnectionPill state={syncState} />
          </div>
        </header>
        <div className="narrative-content">
          {!selectedSession ? (
            <EmptySession
              onCreate={() => void createSession().catch(() => undefined)}
              pending={catalog.createPending}
            />
          ) : snapshot.isError ? (
            <StatePanel
              title="Session could not be loaded"
              detail={messageOf(snapshot.error)}
              action="Try again"
              onAction={() => void snapshot.refetch()}
            />
          ) : (
            <>
              <SessionOverview
                session={selectedSession}
                itemCount={snapshot.data?.items.length}
                runCount={snapshot.data?.runs.length}
                pending={snapshot.isPending}
              />
              {goalsEnabled && snapshot.data ? (
                <GoalComposer
                  key={selectedSession.id}
                  sessionId={selectedSession.id}
                  goal={snapshot.data.goal}
                  actions={goalActions}
                />
              ) : null}
            </>
          )}
        </div>
      </section>

      <aside className="context-dock" aria-labelledby="context-title">
        <header className="panel-header window-drag">
          <div>
            <span className="eyebrow">Context Dock</span>
            <h2 id="context-title">Session</h2>
          </div>
        </header>
        {selectedSession && goalsEnabled ? (
          <GoalTray
            key={selectedSession.id}
            sessionId={selectedSession.id}
            goal={snapshot.data?.goal}
            pending={snapshot.isPending}
            error={snapshot.error}
            actions={goalActions}
          />
        ) : null}
        <RuntimeFacts connection={connection} discovery={props.discovery} />
      </aside>
    </main>
  );
}

function EmptySession(props: { onCreate: () => void; pending: boolean }) {
  return (
    <section className="narrative-empty">
      <div className="orb" aria-hidden="true"><span /></div>
      <h3>Start with a clean session.</h3>
      <p>
        A session binds conversation, plan, goal, and recovery facts to one exact
        workspace identity.
      </p>
      <button
        className="primary-action"
        type="button"
        disabled={props.pending}
        onClick={props.onCreate}
      >
        {props.pending ? "Creating…" : "New session"}
      </button>
    </section>
  );
}

function SessionOverview(props: {
  session: Session;
  itemCount: number | undefined;
  runCount: number | undefined;
  pending: boolean;
}) {
  return (
    <section className="session-overview" aria-busy={props.pending}>
      <span className="eyebrow">Mounted session</span>
      <h3>{props.session.title || "Untitled session"}</h3>
      <p>
        {props.pending
          ? "Loading the coherent session snapshot…"
          : `${props.itemCount ?? 0} narrative items across ${props.runCount ?? 0} runs are mounted from the Runtime.`}
      </p>
    </section>
  );
}

function StatePanel(props: {
  title: string;
  detail: string;
  action: string;
  onAction: () => void;
}) {
  return (
    <section className="state-panel" role="alert">
      <h3>{props.title}</h3>
      <p>{props.detail}</p>
      <button className="secondary-action" type="button" onClick={props.onAction}>
        {props.action}
      </button>
    </section>
  );
}

function ConnectionPill({ state }: { state: RuntimeSyncState }) {
  const label =
    state === "live"
      ? "Live updates"
      : state === "retrying"
        ? "Reconnecting"
        : state === "connecting"
          ? "Connecting"
          : "Runtime online";
  return (
    <span className="online-pill" data-state={state}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

function RuntimeFacts(props: {
  connection: RuntimeConnection;
  discovery: DiscoverResponse;
}) {
  const enabledFeatures = Object.values(
    props.discovery.capabilities.features,
  ).filter((feature) => feature.enabled).length;
  return (
    <section className="runtime-section" aria-labelledby="runtime-facts-title">
      <h3 id="runtime-facts-title">Runtime</h3>
      <dl className="runtime-facts">
        <Fact label="Generation" value={String(props.connection.generation)} numeric />
        <Fact label="Protocol" value={props.discovery.protocolVersion} />
        <Fact label="Features online" value={String(enabledFeatures)} numeric />
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
    </section>
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

function selectSession(
  sessions: Session[],
  selectedId: string | undefined,
): Session | undefined {
  return sessions.find((session) => session.id === selectedId) ?? sessions[0];
}

function shortIdentity(identity: string): string {
  return identity.length <= 18
    ? identity
    : `${identity.slice(0, 10)}…${identity.slice(-6)}`;
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "An unknown Runtime error occurred.";
}
