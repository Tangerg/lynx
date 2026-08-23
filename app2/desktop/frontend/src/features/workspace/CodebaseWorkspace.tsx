import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { FormEvent } from "react";

import type {
  CodebaseHit,
  CodebaseStatus,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

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
    return <CodebaseState title="Reading semantic index" />;
  }
  if (status.isError) {
    return (
      <CodebaseState
        title="Semantic index is unavailable"
        detail={messageOf(status.error)}
        action="Try again"
        onAction={() => void status.refetch()}
      />
    );
  }

  return (
    <section className="codebase-workspace" aria-label="Semantic codebase search">
      <CodebaseHeader
        status={status.data}
        rebuilding={reindex.isPending}
        onReindex={() => reindex.mutate()}
      />
      {reindex.isError ? (
        <p className="codebase-error" role="alert">
          {messageOf(reindex.error)}
        </p>
      ) : null}
      {status.data.state === "ready" ? (
        <>
          <form className="codebase-search" role="search" onSubmit={submit}>
            <label htmlFor="codebase-search-input">Ask the codebase</label>
            <div>
              <input
                id="codebase-search-input"
                type="search"
                maxLength={maxCodebaseQueryLength}
                value={props.draft}
                placeholder="Where is session recovery handled?"
                onChange={(event) =>
                  props.onDraftChange(event.currentTarget.value)
                }
              />
              <button type="submit" disabled={props.draft.trim() === ""}>
                Search
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
  return (
    <header className="codebase-header">
      <div>
        <p>Semantic index</p>
        {props.status.state === "ready" ? (
          <small>
            {props.status.fileCount.toLocaleString()} files ·{" "}
            {props.status.chunkCount.toLocaleString()} passages
            {props.status.truncated ? " · bounded" : ""}
          </small>
        ) : (
          <small>{statusLabel(props.status.state)}</small>
        )}
        {props.status.state === "ready" ? (
          <small title={props.status.modelId}>
            {props.status.modelId}
            {props.status.indexedAt
              ? ` · Indexed ${formatIndexedAt(props.status.indexedAt)}`
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
            ? "Indexing…"
            : "Reindex"}
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
  if (props.status.state === "indexing") {
    return (
      <CodebaseState
        title="Building the semantic index"
        detail="Source discovery, chunking, and embeddings run in the Runtime. This view updates when the durable index settles."
      />
    );
  }
  if (props.status.state === "error") {
    return (
      <CodebaseState
        title="The last index build did not finish"
        detail="Check the embedding provider configuration, then rebuild the index. The previous searchable index is never partially replaced."
        action={props.mutationPending ? "Starting…" : "Build again"}
        disabled={props.mutationPending}
        onAction={props.onReindex}
      />
    );
  }
  return (
    <CodebaseState
      title="Search by meaning, not only text"
      detail="Build a workspace-scoped semantic index to find relevant code passages across the current Session workspace."
      action={props.mutationPending ? "Starting…" : "Build index"}
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
  if (props.query === "") {
    return (
      <CodebaseState
        title="Ready to search"
        detail="Describe a responsibility, behavior, or concept in plain language."
      />
    );
  }
  if (props.pending) {
    return <CodebaseState title="Searching code passages" />;
  }
  if (props.error !== null && props.error !== undefined) {
    return (
      <CodebaseState
        title="Semantic search failed"
        detail={messageOf(props.error)}
      />
    );
  }
  if ((props.hits?.length ?? 0) === 0) {
    return (
      <CodebaseState
        title="No relevant passage found"
        detail="Try a broader description, or rebuild after changing source files."
      />
    );
  }
  return (
    <ol className="codebase-results" aria-label="Semantic search results">
      {props.hits?.map((hit) => (
        <li key={`${hit.path}:${hit.startLine}:${hit.endLine}`}>
          <button
            type="button"
            onClick={() => props.onOpenFile(hit.path, hit.startLine)}
          >
            <span>
              <strong>{hit.path}</strong>
              <small>
                L{hit.startLine}–{hit.endLine} · {Math.round(hit.score * 100)}%
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

function statusLabel(state: CodebaseStatus["state"]) {
  switch (state) {
    case "indexing":
      return "Building index";
    case "error":
      return "Build interrupted";
    case "ready":
      return "Ready";
    default:
      return "Not indexed";
  }
}

function formatIndexedAt(value: string) {
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp)
    ? value
    : new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(timestamp);
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Unexpected Runtime error";
}
