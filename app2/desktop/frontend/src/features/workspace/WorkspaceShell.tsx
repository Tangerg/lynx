import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import type {
  DiscoverResponse,
  RuntimeConnection,
  Session,
} from "@lyra/runtime-contract";

import { GoalComposer } from "../goals/GoalComposer";
import { GoalTray } from "../goals/GoalTray";
import { useGoalActions } from "../goals/useGoalActions";
import { PlanCompact } from "../plan/PlanCompact";
import {
  createSession,
  listSessions,
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
  const queryClient = useQueryClient();
  const [selectedSessionId, setSelectedSessionId] = useState<string>();
  const planEnabled = props.discovery.capabilities.features.plan?.enabled === true;
  const goalsEnabled = props.discovery.capabilities.features.goals?.enabled === true;
  const liveUpdatesEnabled =
    props.discovery.capabilities.streamingMethods.includes("runtime.subscribe") &&
    (["sessions.changed", "plan.changed", "goals.changed"] as const).every(
      (topic) => props.discovery.capabilities.runtimeTopics.includes(topic),
    );
  const syncState = useRuntimeInvalidations(connection, liveUpdatesEnabled);

  const sessions = useQuery({
    queryKey: runtimeQueryKeys.sessions(connection),
    queryFn: ({ signal }) => listSessions(connection, signal),
    retry: 2,
  });
  const selectedSession = selectSession(sessions.data, selectedSessionId);
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
  const create = useMutation({
    mutationFn: () => createSession(connection),
    onSuccess: (session) => {
      queryClient.setQueryData<Session[]>(
        runtimeQueryKeys.sessions(connection),
        (current) => [
          session,
          ...(current ?? []).filter((candidate) => candidate.id !== session.id),
        ],
      );
      setSelectedSessionId(session.id);
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.sessions(connection),
      });
    },
  });

  return (
    <main className="app-shell">
      <aside className="work-index" aria-labelledby="work-index-title">
        <header className="panel-header window-drag">
          <div>
            <span className="eyebrow">Lyra</span>
            <h1 id="work-index-title">Work Index</h1>
          </div>
          <button
            className="icon-action window-no-drag"
            type="button"
            aria-label="New session"
            title="New session"
            disabled={create.isPending}
            onClick={() => create.mutate()}
          >
            <span aria-hidden="true">＋</span>
          </button>
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
        <SessionList
          sessions={sessions.data}
          selectedId={selectedSession?.id}
          pending={sessions.isPending}
          error={sessions.error ?? create.error}
          onSelect={setSelectedSessionId}
          onRetry={() => void sessions.refetch()}
        />
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
            <EmptySession onCreate={() => create.mutate()} pending={create.isPending} />
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

function SessionList(props: {
  sessions: Session[] | undefined;
  selectedId: string | undefined;
  pending: boolean;
  error: unknown;
  onSelect: (sessionId: string) => void;
  onRetry: () => void;
}) {
  if (props.pending) {
    return <p className="panel-note" aria-busy="true">Loading sessions…</p>;
  }
  if (props.error) {
    return (
      <div className="panel-error" role="alert">
        <p>{messageOf(props.error)}</p>
        <button className="quiet-action" type="button" onClick={props.onRetry}>
          Retry
        </button>
      </div>
    );
  }
  if (!props.sessions || props.sessions.length === 0) {
    return <p className="panel-note">No sessions in this workspace yet.</p>;
  }
  return (
    <nav className="session-list" aria-label="Sessions">
      {props.sessions.map((session) => (
        <button
          className="session-row"
          data-selected={session.id === props.selectedId}
          type="button"
          key={session.id}
          onClick={() => props.onSelect(session.id)}
        >
          <span className="session-row-main">
            <strong>{session.title || "Untitled session"}</strong>
            <small>{compactPath(session.workspace.ref.path)}</small>
          </span>
          <span className="session-row-meta">
            <span className="session-state" data-status={session.status}>
              {sessionStatus(session.status)}
            </span>
            <time dateTime={session.updatedAt}>{formatUpdatedAt(session.updatedAt)}</time>
          </span>
        </button>
      ))}
    </nav>
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
  sessions: Session[] | undefined,
  selectedId: string | undefined,
): Session | undefined {
  return sessions?.find((session) => session.id === selectedId) ?? sessions?.[0];
}

function sessionStatus(status: string): string {
  if (status === "running") return "Running";
  if (status === "waiting") return "Waiting";
  if (status === "idle") return "Idle";
  return status;
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

function formatUpdatedAt(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? ""
    : new Intl.DateTimeFormat(undefined, {
        month: "short",
        day: "numeric",
      }).format(date);
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "An unknown Runtime error occurred.";
}
