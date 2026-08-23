import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  DiscoverResponse,
  RuntimeConnection,
  Session,
} from "@lyra/runtime-contract";

import { AgentNarrative } from "../agent/AgentNarrative";
import { SessionModelPicker } from "../agent/SessionModelPicker";
import {
  Composer,
  emptyComposerDraft,
  type ComposerDraft,
} from "../agent/Composer";
import { useAgentSessionView } from "../agent/useAgentSessionView";
import { ContextDock } from "./ContextDock";
import { GoalComposer } from "../goals/GoalComposer";
import { GoalTray } from "../goals/GoalTray";
import { useGoalActions } from "../goals/useGoalActions";
import { PlanCompact } from "../plan/PlanCompact";
import { SettingsSurface } from "../settings/SettingsSurface";
import { NewSessionMenu } from "../sessions/NewSessionMenu";
import { SessionIndex } from "../sessions/SessionIndex";
import { compactPath } from "../sessions/sessionPresentation";
import { useSessionCatalog } from "../sessions/useSessionCatalog";
import {
  listModels,
  listRecipes,
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
  const [composerDrafts, setComposerDrafts] = useState<
    Record<string, ComposerDraft>
  >({});
  const [dockExpanded, setDockExpanded] = useState(false);
	const [settingsOpen, setSettingsOpen] = useState(false);
  const planEnabled = props.discovery.capabilities.features.plan?.enabled === true;
  const goalsEnabled = props.discovery.capabilities.features.goals?.enabled === true;
  const skillsEnabled = props.discovery.capabilities.features.skills?.enabled === true;
  const knowledgeEnabled =
    props.discovery.capabilities.features.knowledge?.enabled === true;
  const memoryEnabled =
    props.discovery.capabilities.features.agentMemory?.enabled === true;
  const liveUpdatesEnabled =
    props.discovery.capabilities.streamingMethods.includes("runtime.subscribe") &&
    (
      [
        "sessions.changed",
        "runs.changed",
        "plan.changed",
        "goals.changed",
        "interrupts.changed",
        "models.changed",
        "mcp.changed",
		"approvals.changed",
		"schedules.changed",
		"hooks.changed",
        "files.changed",
        "skills.changed",
        "knowledge.changed",
        "agentMemory.changed",
        "codebase.changed",
      ] as const
    ).every((topic) =>
      props.discovery.capabilities.runtimeTopics.includes(topic),
    );
  const catalog = useSessionCatalog(connection);
  const selectedSession = selectSession(catalog.sessions, selectedSessionId);
  const fileWatch = useMemo(
    () =>
      props.discovery.capabilities.features.fileWatch?.enabled === true &&
      selectedSession?.workspace.availability === "available"
        ? {
            id: `session:${selectedSession.id}`,
            workspace: selectedSession.workspace.ref,
          }
        : undefined,
    [
      props.discovery.capabilities.features.fileWatch?.enabled,
      selectedSession?.id,
      selectedSession?.workspace.availability,
      selectedSession?.workspace.ref.path,
    ],
  );
  const syncState = useRuntimeInvalidations(
    connection,
    liveUpdatesEnabled,
    fileWatch,
  );
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
	const durableSelection =
		selectedSession?.provider && selectedSession.model
			? { provider: selectedSession.provider, model: selectedSession.model }
			: undefined;
  const agentView = useAgentSessionView(
    connection,
    selectedSession?.id,
    snapshot.data,
		durableSelection,
  );
	const modelProvider =
		agentView.focusRootRun?.provider ?? selectedSession?.provider;
  const modelCatalog = useQuery({
    queryKey: runtimeQueryKeys.models(
      connection,
		  modelProvider ?? "unselected",
    ),
    queryFn: ({ signal }) =>
      listModels(
        connection,
		    modelProvider ?? "",
        signal,
      ),
		enabled: modelProvider !== undefined,
    staleTime: 5 * 60_000,
  });
  const recipes = useQuery({
    queryKey: runtimeQueryKeys.workspaceRecipes(
      connection,
      selectedSession?.workspace.ref.path ?? "unselected",
    ),
    queryFn: ({ signal }) =>
      listRecipes(
        connection,
        selectedSession?.workspace.ref ?? { path: "" },
        signal,
      ),
    enabled: selectedSession?.workspace.availability === "available",
    retry: 2,
  });
  const contextModel = modelCatalog.data?.data.find(
		(model) =>
			model.id === (agentView.focusRootRun?.model ?? selectedSession?.model),
  );
  const composerDraft = selectedSession
    ? (composerDrafts[selectedSession.id] ?? emptyComposerDraft)
    : emptyComposerDraft;
  const updateComposerDraft = useCallback(
    (update: (draft: ComposerDraft) => ComposerDraft) => {
      if (!selectedSession) return;
      setComposerDrafts((current) => {
        const draft = current[selectedSession.id] ?? emptyComposerDraft;
        return {
          ...current,
          [selectedSession.id]: update(draft),
        };
      });
    },
    [selectedSession],
  );
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
      setComposerDrafts((current) => {
        if (current[session.id] === undefined) return current;
        const next = { ...current };
        delete next[session.id];
        return next;
      });
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
		<>
	<main className="app-shell" aria-hidden={settingsOpen || undefined} inert={settingsOpen}>
      <aside className="work-index" aria-labelledby="work-index-title">
        <header className="panel-header window-drag">
          <div>
            <span className="eyebrow">Lyra</span>
            <h1 id="work-index-title">Work Index</h1>
          </div>
		  <div className="work-index-actions window-no-drag">
			<button className="icon-action" type="button" aria-label="Open settings" title="Settings" onClick={() => setSettingsOpen(true)}>⚙</button>
			<NewSessionMenu
			  pending={catalog.createPending}
			  defaultWorkspace={props.discovery.serverInfo.defaultWorkspace.path}
			  onCreate={createSession}
			/>
		  </div>
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
                plan={agentView.plan}
                pending={snapshot.isPending}
                error={snapshot.isError}
              />
            ) : null}
            <ContextGauge
              tokens={agentView.contextTokens}
              contextWindow={contextModel?.contextWindow}
			  model={
				contextModel?.displayName ??
				agentView.focusRootRun?.model ??
				selectedSession?.model
			  }
            />
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
          ) : snapshot.isPending || snapshot.data === undefined ? (
            <SessionLoading title={selectedSession.title} />
          ) : (
            <>
              <AgentNarrative
                key={`narrative:${selectedSession.id}`}
                sessionTitle={selectedSession.title}
                items={agentView.items}
                runs={agentView.runs}
                liveToolOutputs={agentView.liveToolOutputs}
                interrupts={agentView.interrupts}
                progress={agentView.progress}
                pending={snapshot.isFetching || agentView.actionPending}
                interruptPending={agentView.actionPending}
                interruptError={agentView.interruptError}
                streamError={agentView.streamError}
                cancelingRunId={agentView.cancelingRunId}
                cancelError={agentView.cancelError}
                onResume={agentView.resume}
                onCancelRun={agentView.cancel}
              >
                {goalsEnabled ? (
                  <GoalComposer
                    key={selectedSession.id}
                    sessionId={selectedSession.id}
                    goal={snapshot.data.goal}
                    actions={goalActions}
                  />
                ) : null}
              </AgentNarrative>
              <Composer
                key={`composer:${selectedSession.id}`}
                sessionId={selectedSession.id}
                draft={composerDraft}
                activeRun={agentView.activeRootRun}
                recipes={recipes.data?.data ?? []}
                pending={agentView.actionPending}
                error={agentView.actionError}
				attachmentPolicy={
					contextModel?.capabilities?.multimodal === true
						? "multimodal"
						: "text-only"
				}
                onChange={updateComposerDraft}
                onSend={agentView.send}
                onStop={agentView.stop}
			  >
				<SessionModelPicker
					connection={connection}
					session={selectedSession}
					disabled={agentView.activeRootRun !== undefined || catalog.updatePending}
					onChange={(provider, model) =>
						catalog.update({
							source: selectedSession,
							patch: { provider, model },
						})
					}
				/>
			  </Composer>
            </>
          )}
        </div>
      </section>

      <aside
        className="context-dock"
        data-expanded={dockExpanded}
        aria-labelledby="context-title"
      >
        <header className="panel-header window-drag">
          <div>
            <span className="eyebrow">Context Dock</span>
            <h2 id="context-title">Session</h2>
          </div>
        </header>
        <ContextDock
          connection={connection}
          session={selectedSession}
          runs={agentView.runs}
          items={agentView.items}
          interrupts={agentView.interrupts}
          liveToolOutputs={agentView.liveToolOutputs}
          actionPending={agentView.actionPending}
          cancelingRunId={agentView.cancelingRunId}
          cancelError={agentView.cancelError}
          onExpandedChange={setDockExpanded}
          onCancelRun={agentView.cancel}
          skillsEnabled={skillsEnabled}
          knowledgeEnabled={knowledgeEnabled}
          memoryEnabled={memoryEnabled}
        >
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
        </ContextDock>
      </aside>
	</main>
		{settingsOpen ? (
			<SettingsSurface
				connection={connection}
				sessionId={selectedSession?.id}
				workspace={selectedSession?.workspace.ref}
				onClose={() => setSettingsOpen(false)}
				onOpenSession={(sessionId) => {
					setSelectedSessionId(sessionId);
					setSettingsOpen(false);
				}}
			/>
		) : null}
		</>
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

function SessionLoading({ title }: { title: string }) {
  return (
    <section className="session-overview" aria-busy="true">
      <span className="eyebrow">Mounting session</span>
      <h3>{title || "Untitled session"}</h3>
      <p>Loading one coherent Runtime snapshot before accepting new work…</p>
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

function ContextGauge(props: {
  tokens: number | undefined;
  contextWindow: number | undefined;
  model: string | undefined;
}) {
  if (props.tokens === undefined || props.tokens <= 0) return null;
  const ratio =
    props.contextWindow && props.contextWindow > 0
      ? Math.min(props.tokens / props.contextWindow, 1)
      : undefined;
  const label =
    ratio === undefined
      ? `${formatTokens(props.tokens)} context tokens`
      : `${Math.round(ratio * 100)}% of ${formatTokens(props.contextWindow ?? 0)} context`;
  return (
    <div className="context-gauge" title={props.model} aria-label={label}>
      <span>
        <i
          style={
            ratio === undefined ? undefined : { width: `${ratio * 100}%` }
          }
        />
      </span>
      <b>
        {ratio === undefined
          ? formatTokens(props.tokens)
          : `${Math.round(ratio * 100)}%`}
      </b>
      <small>ctx</small>
    </div>
  );
}

function formatTokens(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
  return value.toLocaleString();
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
