import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import type {
  KnowledgeEntry,
  Page,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  listKnowledge,
  runtimeQueryKeys,
  updateKnowledge,
} from "../../runtime/runtimeQueries";
import { ResourceState } from "./ResourceState";

interface KnowledgeWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  enabled: boolean;
}

export function KnowledgeWorkspace(props: KnowledgeWorkspaceProps) {
  const query = useQuery({
    queryKey: runtimeQueryKeys.workspaceKnowledge(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listKnowledge(props.connection, props.workspace, signal),
    enabled: props.enabled,
    retry: 2,
  });

  if (!props.enabled) {
    return (
      <ResourceState
        title="Knowledge unavailable"
        detail="This Runtime does not advertise the Lyra Knowledge capability."
      />
    );
  }
  if (query.isPending) return <ResourceState title="Loading Knowledge…" />;
  if (query.error) {
    return (
      <ResourceState
        title="Knowledge could not be loaded"
        detail={messageOf(query.error)}
        action="Try again"
        onAction={() => void query.refetch()}
      />
    );
  }
  if (!query.data || query.data.data.length === 0) {
    return (
      <ResourceState
        title="No Knowledge scopes available"
        detail="Lyra could not resolve a writable LYRA.md scope for this workspace."
      />
    );
  }

  return (
    <div className="knowledge-document-list" aria-label="Knowledge documents">
      {query.data.data.map((entry) => (
        <KnowledgeEditor
          key={`${props.connection.generation}:${props.workspace.path}:${entry.scope}`}
          connection={props.connection}
          workspace={props.workspace}
          entry={entry}
          queryKey={runtimeQueryKeys.workspaceKnowledge(
            props.connection,
            props.workspace.path,
          )}
          onRefresh={async () => (await query.refetch()).data}
        />
      ))}
    </div>
  );
}

interface KnowledgeDraft {
  baseline: string;
  value: string;
  revision: string;
  updatedAt?: string;
}

function KnowledgeEditor(props: {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  entry: KnowledgeEntry;
  queryKey: readonly unknown[];
  onRefresh(): Promise<Page<KnowledgeEntry> | undefined>;
}) {
  const queryClient = useQueryClient();
  const controller = useRef<AbortController | undefined>(undefined);
  const saving = useRef(false);
  const [draft, setDraft] = useState<KnowledgeDraft>(() => openDraft(props.entry));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string>();
  const dirty = draft.value !== draft.baseline;

  useEffect(() => {
    setDraft((current) =>
      current.value === current.baseline ? openDraft(props.entry) : current,
    );
  }, [props.entry.content, props.entry.revision, props.entry.updatedAt]);
  useEffect(
    () => () => {
      const active = controller.current;
      controller.current = undefined;
      active?.abort();
    },
    [],
  );

  const save = async () => {
    if (!dirty || saving.current) return;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    saving.current = true;
    setPending(true);
    setError(undefined);
    const submitted = draft.value;
    const expectedRevision = draft.revision;
    try {
      const saved = await updateKnowledge(
        props.connection,
        {
          scope: props.entry.scope,
          workspace: props.workspace,
          expectedRevision,
          content: submitted,
        },
        request.signal,
      );
      if (request.signal.aborted) return;
      setDraft(openDraft(saved));
      queryClient.setQueryData<Page<KnowledgeEntry>>(props.queryKey, (current) =>
        current === undefined
          ? current
          : {
              ...current,
              data: current.data.map((entry) =>
                entry.scope === saved.scope ? saved : entry,
              ),
            },
      );
      await queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.knowledge(props.connection),
      });
    } catch (cause) {
      if (request.signal.aborted) return;
      let message = messageOf(cause);
      try {
        const latestPage = await props.onRefresh();
        const latest = latestPage?.data.find(
          (entry) => entry.scope === props.entry.scope,
        );
        if (latest && latest.revision !== expectedRevision) {
          if (latest.content === submitted) {
            setDraft(openDraft(latest));
            message = "";
          } else {
            setDraft((current) => ({
              ...openDraft(latest),
              value: current.value,
            }));
            message =
              "This Knowledge document changed externally. Your draft is preserved against the latest revision; review it before saving again.";
          }
        }
      } catch {
        // The original write failure remains the actionable error.
      }
      setError(message || undefined);
    } finally {
      if (controller.current === request) {
        controller.current = undefined;
        saving.current = false;
        setPending(false);
      }
    }
  };

  return (
    <article className="knowledge-document">
      <header>
        <div>
          <h4>{knowledgePath(props.entry.scope)}</h4>
          <p>{knowledgeScope(props.entry.scope)}</p>
        </div>
        {draft.updatedAt ? (
          <time dateTime={draft.updatedAt}>
            {new Date(draft.updatedAt).toLocaleString()}
          </time>
        ) : (
          <span>Not created</span>
        )}
      </header>
      <textarea
        value={draft.value}
        rows={10}
        maxLength={1_048_576}
        spellCheck={false}
        aria-label={`${knowledgeScope(props.entry.scope)} Knowledge`}
        onChange={(event) => {
          setError(undefined);
          setDraft((current) => ({ ...current, value: event.currentTarget.value }));
        }}
      />
      <footer>
        <span>{dirty ? "Unsaved changes" : "Saved"}</span>
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => {
            setError(undefined);
            setDraft(openDraft(props.entry));
          }}
        >
          Revert
        </button>
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => void save()}
        >
          {pending ? "Saving…" : "Save"}
        </button>
      </footer>
      {error ? <p className="knowledge-error" role="alert">{error}</p> : null}
    </article>
  );
}

function openDraft(entry: KnowledgeEntry): KnowledgeDraft {
  return {
    baseline: entry.content,
    value: entry.content,
    revision: entry.revision,
    ...(entry.updatedAt === undefined ? {} : { updatedAt: entry.updatedAt }),
  };
}

function knowledgePath(scope: string) {
  if (scope === "home") return "~/.lyra/LYRA.md";
  if (scope === "projectRoot") return "Project root / LYRA.md";
  return "Workspace / LYRA.md";
}

function knowledgeScope(scope: string) {
  if (scope === "home") return "User preferences";
  if (scope === "projectRoot") return "Project Knowledge";
  return "Workspace Knowledge";
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Knowledge operation failed.";
}
