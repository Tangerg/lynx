import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { FormEvent } from "react";

import type {
  CodebaseHit,
  CodebaseStatus,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  useLocalization,
  type Translate,
} from "../localization/Localization";
import {
  getCodebaseStatus,
  reindexCodebase,
  runtimeQueryKeys,
  searchCodebase,
} from "../../runtime/runtimeQueries";
import { maxCodebaseQueryLength } from "./contextDockState";

interface CodebaseWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  draft: string;
  query: string;
  onDraftChange(value: string): void;
  onSubmit(value: string): void;
  onOpenFile(path: string, line?: number): void;
}

export function CodebaseWorkspace(props: CodebaseWorkspaceProps) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const status = useQuery({
    queryKey: runtimeQueryKeys.codebaseStatus(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      getCodebaseStatus(props.connection, props.workspace, signal),
    retry: 1,
  });
  const reindex = useMutation({
    mutationFn: () => reindexCodebase(props.connection, props.workspace),
    onSuccess: (response) => {
      const statusKey = runtimeQueryKeys.codebaseStatus(
        props.connection,
        props.workspace.path,
      );
      queryClient.setQueryData<CodebaseStatus>(statusKey, (current) =>
        current === undefined
          ? current
          : {
              ...current,
              state: "indexing",
              operationId: response.operationId,
            },
      );
      return queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.codebase(
          props.connection,
          props.workspace.path,
        ),
      });
    },
  });
  const search = useQuery({
    queryKey: runtimeQueryKeys.codebaseSearch(
      props.connection,
      props.workspace.path,
      props.query,
    ),
    queryFn: ({ signal }) =>
      searchCodebase(
        props.connection,
        props.workspace,
        props.query,
        signal,
      ),
    enabled: status.data?.state === "ready" && props.query !== "",
    retry: 1,
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    props.onSubmit(props.draft.trim());
  };

  if (status.isPending) {
    return <CodebaseState title={t("codebase.readingIndex")} />;
  }
  if (status.isError) {
    return (
      <CodebaseState
        title={t("codebase.indexUnavailable")}
        detail={messageOf(status.error, t("codebase.runtimeError"))}
        action={t("resource.tryAgain")}
        onAction={() => void status.refetch()}
      />
    );
  }

  return (
    <section
      className="codebase-workspace"
      aria-label={t("codebase.searchLabel")}
    >
      <CodebaseHeader
        status={status.data}
        rebuilding={reindex.isPending}
        onReindex={() => reindex.mutate()}
      />
      {reindex.isError ? (
        <p className="codebase-error" role="alert">
          {messageOf(reindex.error, t("codebase.runtimeError"))}
        </p>
      ) : null}
      {status.data.state === "ready" ? (
        <>
          <form className="codebase-search" role="search" onSubmit={submit}>
            <label htmlFor="codebase-search-input">{t("codebase.ask")}</label>
            <div>
              <input
                id="codebase-search-input"
                type="search"
                maxLength={maxCodebaseQueryLength}
                value={props.draft}
                placeholder={t("codebase.placeholder")}
                onChange={(event) =>
                  props.onDraftChange(event.currentTarget.value)
                }
              />
              <button type="submit" disabled={props.draft.trim() === ""}>
                {t("codebase.search")}
              </button>
            </div>
          </form>
          <CodebaseResults
            query={props.query}
            pending={search.isPending && search.fetchStatus === "fetching"}
            error={search.error}
            hits={search.data?.hits}
            onOpenFile={props.onOpenFile}
          />
        </>
      ) : (
        <IndexLifecycleState
          status={status.data}
          mutationPending={reindex.isPending}
          onReindex={() => reindex.mutate()}
        />
      )}
    </section>
  );
}

function CodebaseHeader(props: {
  status: CodebaseStatus;
  rebuilding: boolean;
  onReindex(): void;
}) {
  const { formatDateTime, formatNumber, t } = useLocalization();
  return (
    <header className="codebase-header">
      <div>
        <p>{t("codebase.index")}</p>
        {props.status.state === "ready" ? (
          <small>
            {t("codebase.indexFacts", {
              files: formatNumber(props.status.fileCount),
              passages: formatNumber(props.status.chunkCount),
            })}
            {props.status.truncated ? ` · ${t("codebase.bounded")}` : ""}
          </small>
        ) : (
          <small>{statusLabel(props.status.state, t)}</small>
        )}
        {props.status.state === "ready" ? (
          <small title={props.status.modelId}>
            {props.status.modelId}
            {props.status.indexedAt
              ? ` · ${t("codebase.indexedAt", {
                  date: formatIndexedAt(props.status.indexedAt, formatDateTime),
                })}`
              : ""}
          </small>
        ) : null}
      </div>
      {props.status.state === "ready" || props.status.state === "indexing" ? (
        <button
          type="button"
          disabled={props.rebuilding || props.status.state === "indexing"}
          onClick={props.onReindex}
        >
          {props.rebuilding || props.status.state === "indexing"
            ? t("codebase.indexing")
            : t("codebase.reindex")}
        </button>
      ) : null}
    </header>
  );
}

function IndexLifecycleState(props: {
  status: CodebaseStatus;
  mutationPending: boolean;
  onReindex(): void;
}) {
  const { t } = useLocalization();
  if (props.status.state === "indexing") {
    return (
      <CodebaseState
        title={t("codebase.building")}
        detail={t("codebase.buildingDetail")}
      />
    );
  }
  if (props.status.state === "error") {
    return (
      <CodebaseState
        title={t("codebase.lastBuildFailed")}
        detail={t("codebase.lastBuildFailedDetail")}
        action={props.mutationPending ? t("codebase.starting") : t("codebase.buildAgain")}
        disabled={props.mutationPending}
        onAction={props.onReindex}
      />
    );
  }
  return (
    <CodebaseState
      title={t("codebase.semanticTitle")}
      detail={t("codebase.semanticDetail")}
      action={props.mutationPending ? t("codebase.starting") : t("codebase.buildIndex")}
      disabled={props.mutationPending}
      onAction={props.onReindex}
    />
  );
}

function CodebaseResults(props: {
  query: string;
  pending: boolean;
  error: unknown;
  hits: CodebaseHit[] | undefined;
  onOpenFile(path: string, line?: number): void;
}) {
  const { t } = useLocalization();
  if (props.query === "") {
    return (
      <CodebaseState
        title={t("codebase.readySearch")}
        detail={t("codebase.readySearchDetail")}
      />
    );
  }
  if (props.pending) {
    return <CodebaseState title={t("codebase.searchingPassages")} />;
  }
  if (props.error !== null && props.error !== undefined) {
    return (
      <CodebaseState
        title={t("codebase.searchFailed")}
        detail={messageOf(props.error, t("codebase.runtimeError"))}
      />
    );
  }
  if ((props.hits?.length ?? 0) === 0) {
    return (
      <CodebaseState
        title={t("codebase.noPassage")}
        detail={t("codebase.noPassageDetail")}
      />
    );
  }
  return (
    <ol className="codebase-results" aria-label={t("codebase.results")}>
      {props.hits?.map((hit) => (
        <li key={`${hit.path}:${hit.startLine}:${hit.endLine}`}>
          <button
            type="button"
            onClick={() => props.onOpenFile(hit.path, hit.startLine)}
          >
            <span>
              <strong>{hit.path}</strong>
              <small>
                {t("codebase.lineRangeScore", {
                  start: hit.startLine,
                  end: hit.endLine,
                  score: Math.round(hit.score * 100),
                })}
              </small>
            </span>
            <pre>{hit.snippet}</pre>
          </button>
        </li>
      ))}
    </ol>
  );
}

function CodebaseState(props: {
  title: string;
  detail?: string;
  action?: string;
  disabled?: boolean;
  onAction?(): void;
}) {
  return (
    <div className="codebase-state">
      <strong>{props.title}</strong>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.action && props.onAction ? (
        <button type="button" disabled={props.disabled} onClick={props.onAction}>
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function statusLabel(state: CodebaseStatus["state"], t: Translate) {
  switch (state) {
    case "indexing":
      return t("codebase.status.building");
    case "error":
      return t("codebase.status.interrupted");
    case "ready":
      return t("codebase.status.ready");
    default:
      return t("codebase.status.notIndexed");
  }
}

function formatIndexedAt(
  value: string,
  formatDateTime: (
    value: Date,
    options?: Intl.DateTimeFormatOptions,
  ) => string,
) {
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp)
    ? value
    : formatDateTime(new Date(timestamp), {
        dateStyle: "medium",
        timeStyle: "short",
      });
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
