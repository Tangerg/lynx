import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";

import type {
  MCPServer,
  MCPTool,
  RuntimeConnection,
} from "@lyra/runtime-contract";

import {
  authorizeMCPServer,
  createMCPServer,
  deleteMCPServer,
  listMCPServers,
  listMCPTools,
  reconnectMCPServer,
  runtimeQueryKeys,
  testMCPServer,
  updateMCPServer,
} from "../../runtime/runtimeQueries";
import { useLocalization, type Translate } from "../localization/Localization";
import { MCPConnectionFields } from "./MCPConnectionFields";
import {
  candidateFromDraft,
  draftFromServer,
  durableMCPServerSignature,
  mcpConnectionSummary,
  mcpDraftChanged,
  newMCPDraft,
  requestFromDraft,
  withToolPolicy,
  MCPDraftValidationError,
  type MCPDraft,
} from "./mcpDraft";

interface MCPSettingsProps {
  connection: RuntimeConnection;
}

type Verdict = { tone: "ok" | "error"; message: string };

export function MCPSettings(props: MCPSettingsProps) {
  const { t } = useLocalization();
  const servers = useQuery({
    queryKey: runtimeQueryKeys.mcpServers(props.connection),
    queryFn: ({ signal }) => listMCPServers(props.connection, signal),
    retry: 2,
  });

  return (
    <>
      <section className="settings-section" aria-labelledby="mcp-add-title">
        <header>
          <div>
            <h2 id="mcp-add-title">{t("settings.mcp.addServer")}</h2>
            <p>{t("settings.mcp.addServerDetail")}</p>
          </div>
        </header>
        <NewMCPServer connection={props.connection} />
      </section>
      <section
        className="settings-section"
        aria-labelledby="mcp-connections-title"
      >
        <header>
          <div>
            <h2 id="mcp-connections-title">
              {t("settings.mcp.configuredServers")}
            </h2>
            <p>{t("settings.mcp.configuredServersDetail")}</p>
          </div>
        </header>
        {servers.isPending ? (
          <SettingsState>{t("settings.mcp.loadingServers")}</SettingsState>
        ) : servers.isError ? (
          <SettingsState
            action={t("settings.common.tryAgain")}
            onAction={() => void servers.refetch()}
          >
            {messageOf(servers.error, t)}
          </SettingsState>
        ) : (servers.data?.data.length ?? 0) === 0 ? (
          <SettingsState>{t("settings.mcp.empty")}</SettingsState>
        ) : (
          <div className="mcp-server-list">
            {servers.data?.data.map((server) => (
              <MCPServerCard
                key={server.name}
                connection={props.connection}
                server={server}
              />
            ))}
          </div>
        )}
      </section>
    </>
  );
}

function NewMCPServer(props: MCPSettingsProps) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<MCPDraft>(newMCPDraft);
  const [verdict, setVerdict] = useState<Verdict>();
  const testController = useRef<AbortController | undefined>(undefined);

  useEffect(() => () => testController.current?.abort(), []);
  const updateDraft = (next: MCPDraft) => {
    testController.current?.abort();
    setVerdict(undefined);
    setDraft(next);
  };
  const create = useMutation({
    mutationFn: () =>
      createMCPServer(props.connection, candidateFromDraft(draft)),
    onSuccess: () => {
      setDraft(newMCPDraft());
      setVerdict(undefined);
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.mcp(props.connection),
      });
    },
  });
  const test = useMutation({
    mutationFn: () => {
      testController.current?.abort();
      const controller = new AbortController();
      testController.current = controller;
      return testMCPServer(
        props.connection,
        candidateFromDraft(draft),
        controller.signal,
      );
    },
    onMutate: () => setVerdict(undefined),
    onSuccess: (result) =>
      setVerdict(
        result.ok
          ? { tone: "ok", message: t("settings.mcp.candidateConnected") }
          : { tone: "error", message: problemMessage(result.error, t) },
      ),
  });

  return (
    <article className="mcp-editor mcp-editor-new">
      <MCPConnectionFields draft={draft} onChange={updateDraft} includeName />
      <footer className="mcp-editor-actions">
        <div aria-live="polite">
          {verdict ? (
            <span className="provider-verdict" data-tone={verdict.tone}>
              {verdict.message}
            </span>
          ) : null}
          {test.isError && !isAbortError(test.error) ? (
            <span className="settings-inline-error">
              {messageOf(test.error, t)}
            </span>
          ) : null}
          {create.isError ? (
            <span className="settings-inline-error">
              {messageOf(create.error, t)}
            </span>
          ) : null}
        </div>
        <div>
          <button
            className="secondary-action"
            type="button"
            disabled={test.isPending || create.isPending}
            onClick={() => test.mutate()}
          >
            {test.isPending
              ? t("settings.mcp.testing")
              : t("settings.mcp.testCandidate")}
          </button>
          <button
            className="primary-action"
            type="button"
            disabled={create.isPending || test.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending
              ? t("settings.mcp.adding")
              : t("settings.mcp.addServer")}
          </button>
        </div>
      </footer>
    </article>
  );
}

function MCPServerCard(props: MCPSettingsProps & { server: MCPServer }) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(() => draftFromServer(props.server));
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [authorizationVerdict, setAuthorizationVerdict] = useState<Verdict>();
  const authorizationController = useRef<AbortController | undefined>(
    undefined,
  );
  const configurationSignature = durableMCPServerSignature(props.server);
  const currentServer = useRef(props.server);
  currentServer.current = props.server;

  useEffect(() => {
    setDraft(draftFromServer(currentServer.current));
    setConfirmDelete(false);
    setAuthorizationVerdict(undefined);
  }, [configurationSignature]);
  useEffect(() => () => authorizationController.current?.abort(), []);

  const tools = useQuery({
    queryKey: runtimeQueryKeys.mcpTools(props.connection, props.server.name),
    queryFn: ({ signal }) =>
      listMCPTools(props.connection, props.server.name, signal),
    enabled: props.server.status.type === "connected",
    retry: 1,
  });
  const changed = mcpDraftChanged(props.server, draft);
  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.mcp(props.connection),
    });
  const save = useMutation({
    mutationFn: () =>
      updateMCPServer(props.connection, requestFromDraft(props.server, draft)),
    onSuccess: (committed) => {
      setDraft(draftFromServer(committed));
      void invalidate();
    },
  });
  const reconnect = useMutation({
    mutationFn: () => reconnectMCPServer(props.connection, props.server.name),
    onSuccess: () => void invalidate(),
  });
  const remove = useMutation({
    mutationFn: () => deleteMCPServer(props.connection, props.server.name),
    onSuccess: () => void invalidate(),
  });
  const authorize = useMutation({
    mutationFn: () => {
      authorizationController.current?.abort();
      const controller = new AbortController();
      authorizationController.current = controller;
      return authorizeMCPServer(
        props.connection,
        props.server.name,
        controller.signal,
      );
    },
    onMutate: () => setAuthorizationVerdict(undefined),
    onSuccess: (attempt) => {
      setAuthorizationVerdict(
        attempt.status.type === "succeeded"
          ? { tone: "ok", message: t("settings.mcp.authorizationCompleted") }
          : {
              tone: "error",
              message:
                attempt.status.type === "canceled"
                  ? t("settings.mcp.authorizationCanceled")
                  : problemMessage(attempt.status.error, t),
            },
      );
      void invalidate();
    },
  });
  const busy =
    save.isPending ||
    reconnect.isPending ||
    remove.isPending ||
    authorize.isPending;
  const failure = mutationError(
    t,
    save.error,
    reconnect.error,
    remove.error,
    authorize.error,
  );

  return (
    <article className="mcp-editor" data-status={props.server.status.type}>
      <header className="mcp-server-heading">
        <div>
          <h3>{props.server.name}</h3>
          <code>{mcpConnectionSummary(props.server)}</code>
        </div>
        <ServerStatus server={props.server} />
      </header>
      <MCPConnectionFields
        draft={draft}
        onChange={setDraft}
        masked={{
          authorization: props.server.connection.authorizationMasked,
          headers: props.server.connection.headersMasked,
          environment: props.server.connection.envMasked,
        }}
      />
      <ToolPolicies
        tools={tools.data?.data ?? []}
        pending={tools.isPending && props.server.status.type === "connected"}
        error={tools.error}
        draft={draft}
        onChange={setDraft}
      />
      {props.server.status.type === "needsAuth" ? (
        <div className="mcp-auth-callout">
          <div>
            <strong>{t("settings.mcp.interactiveAuthorization")}</strong>
            <p>{problemMessage(props.server.status.error, t)}</p>
          </div>
          <button
            className="primary-action"
            type="button"
            disabled={busy || changed}
            title={changed ? t("settings.mcp.saveBeforeAuthorize") : undefined}
            onClick={() => authorize.mutate()}
          >
            {authorize.isPending
              ? t("settings.mcp.waitingAuthorization")
              : t("settings.mcp.authorize")}
          </button>
        </div>
      ) : null}
      {authorizationVerdict ? (
        <p
          className="provider-verdict"
          data-tone={authorizationVerdict.tone}
          aria-live="polite"
        >
          {authorizationVerdict.message}
        </p>
      ) : null}
      <footer className="mcp-editor-actions">
        <div>
          {confirmDelete ? (
            <span className="mcp-delete-confirm">
              {t("settings.mcp.deletePermanently")}
              <button
                type="button"
                className="text-action"
                onClick={() => setConfirmDelete(false)}
              >
                {t("settings.common.keep")}
              </button>
              <button
                type="button"
                className="text-action danger"
                disabled={remove.isPending}
                onClick={() => remove.mutate()}
              >
                {remove.isPending
                  ? t("settings.common.deleting")
                  : t("settings.common.delete")}
              </button>
            </span>
          ) : (
            <button
              className="text-action danger"
              type="button"
              disabled={busy}
              onClick={() => setConfirmDelete(true)}
            >
              {t("settings.mcp.deleteServer")}
            </button>
          )}
          {failure ? (
            <span className="settings-inline-error" role="alert">
              {failure}
            </span>
          ) : null}
        </div>
        <div>
          <button
            className="secondary-action"
            type="button"
            disabled={
              busy || changed || props.server.status.type === "disabled"
            }
            title={changed ? t("settings.mcp.saveBeforeReconnect") : undefined}
            onClick={() => reconnect.mutate()}
          >
            {reconnect.isPending
              ? t("settings.mcp.reconnecting")
              : t("settings.mcp.reconnect")}
          </button>
          <button
            className="primary-action"
            type="button"
            disabled={busy || !changed}
            onClick={() => save.mutate()}
          >
            {save.isPending
              ? t("settings.common.saving")
              : t("settings.common.saveChanges")}
          </button>
        </div>
      </footer>
    </article>
  );
}

function ToolPolicies(props: {
  tools: MCPTool[];
  pending: boolean;
  error: unknown;
  draft: MCPDraft;
  onChange: (draft: MCPDraft) => void;
}) {
  const { t, formatNumber } = useLocalization();
  const names = useMemo(
    () =>
      Array.from(
        new Set([
          ...props.tools.map((tool) => tool.name),
          ...props.draft.disabledTools,
          ...props.draft.autoApproveTools,
        ]),
      ).sort(),
    [props.draft.autoApproveTools, props.draft.disabledTools, props.tools],
  );
  if (props.pending)
    return <p className="mcp-tool-state">{t("settings.mcp.loadingTools")}</p>;
  if (props.error && names.length === 0)
    return (
      <p className="mcp-tool-state" data-error="true">
        {messageOf(props.error, t)}
      </p>
    );
  if (names.length === 0) return null;

  return (
    <section
      className="mcp-tool-policies"
      aria-label={t("settings.mcp.toolPolicies")}
    >
      <header>
        <div>
          <strong>{t("settings.mcp.toolTrust")}</strong>
          <p>{t("settings.mcp.toolTrustDetail")}</p>
        </div>
        <span>
          {t(
            names.length === 1
              ? "settings.mcp.toolCountOne"
              : "settings.mcp.toolCountMany",
            { count: formatNumber(names.length) },
          )}
        </span>
      </header>
      {props.error ? (
        <p className="mcp-tool-state" data-error="true">
          {t("settings.mcp.toolsRefreshFailed")}
        </p>
      ) : null}
      <div>
        {names.map((name) => {
          const policy = props.draft.disabledTools.includes(name)
            ? "disabled"
            : props.draft.autoApproveTools.includes(name)
              ? "autoApprove"
              : "default";
          const tool = props.tools.find((candidate) => candidate.name === name);
          return (
            <label key={name}>
              <span>
                <code>{name}</code>
                {tool?.description ? <small>{tool.description}</small> : null}
              </span>
              <select
                value={policy}
                onChange={(event) =>
                  props.onChange(
                    withToolPolicy(
                      props.draft,
                      name,
                      event.currentTarget.value,
                    ),
                  )
                }
              >
                <option value="default">
                  {t("settings.mcp.askWhenNeeded")}
                </option>
                <option value="disabled">{t("settings.mcp.disabled")}</option>
                <option value="autoApprove">
                  {t("settings.mcp.autoApprove")}
                </option>
              </select>
            </label>
          );
        })}
      </div>
    </section>
  );
}

function ServerStatus({ server }: { server: MCPServer }) {
  const { t, formatNumber } = useLocalization();
  const label =
    server.status.type === "connected" && server.status.toolCount !== undefined
      ? t(
          server.status.toolCount === 1
            ? "settings.mcp.connectedToolOne"
            : "settings.mcp.connectedToolMany",
          { count: formatNumber(server.status.toolCount) },
        )
      : statusLabel(server.status.type, t);
  return (
    <div className="mcp-status-block">
      <span className="mcp-status" data-status={server.status.type}>
        <i aria-hidden="true" />
        {label}
      </span>
      {server.status.error ? (
        <small>{problemMessage(server.status.error, t)}</small>
      ) : null}
    </div>
  );
}

function statusLabel(status: string, t: Translate) {
  switch (status) {
    case "disabled":
      return t("settings.mcp.status.disabled");
    case "disconnected":
      return t("settings.mcp.status.disconnected");
    case "connecting":
      return t("settings.mcp.status.connecting");
    case "connected":
      return t("settings.mcp.status.connected");
    case "failed":
      return t("settings.mcp.status.failed");
    case "needsAuth":
      return t("settings.mcp.status.needsAuth");
    default:
      return status;
  }
}

function problemMessage(
  problem: { type: string; detail?: string } | undefined,
  t: Translate,
) {
  if (problem?.detail) return problem.detail;
  switch (problem?.type) {
    case "mcp_authorization_required":
      return t("settings.mcp.problem.authorizationRequired");
    case "mcp_authorization_failed":
      return t("settings.mcp.problem.authorizationFailed");
    case "mcp_dial_failed":
      return t("settings.mcp.problem.dialFailed");
    case "timeout":
      return t("settings.mcp.problem.timeout");
    default:
      return t("settings.mcp.problem.failed");
  }
}

function mutationError(t: Translate, ...errors: unknown[]) {
  const error = errors.find(Boolean);
  return error === undefined ? "" : messageOf(error, t);
}

function isAbortError(error: unknown) {
  return error instanceof Error && error.name === "AbortError";
}

function SettingsState(props: {
  children: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="settings-state">
      <p>{props.children}</p>
      {props.action && props.onAction ? (
        <button
          className="secondary-action"
          type="button"
          onClick={props.onAction}
        >
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function messageOf(error: unknown, t: Translate) {
  if (error instanceof MCPDraftValidationError) {
    switch (error.code) {
      case "stableNameRequired":
        return t("settings.mcp.validation.stableNameRequired");
      case "endpointRequired":
        return t("settings.mcp.validation.endpointRequired");
      case "commandRequired":
        return t("settings.mcp.validation.commandRequired");
      case "timeoutInvalid":
        return t("settings.mcp.validation.timeoutInvalid");
      case "secretInvalidJSON":
        return t("settings.mcp.validation.secretInvalidJSON", {
          field: secretField(error, t),
        });
      case "secretNotObject":
        return t("settings.mcp.validation.secretNotObject", {
          field: secretField(error, t),
        });
      case "secretInvalidEntries":
        return t("settings.mcp.validation.secretInvalidEntries", {
          field: secretField(error, t),
        });
    }
  }
  return error instanceof Error
    ? error.message
    : t("settings.common.requestFailed");
}

function secretField(error: MCPDraftValidationError, t: Translate) {
  return error.secretKind === "environment"
    ? t("settings.mcp.environment")
    : t("settings.mcp.headers");
}
