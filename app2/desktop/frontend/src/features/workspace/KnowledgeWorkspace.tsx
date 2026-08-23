import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import type {
  KnowledgeEntry,
  Page,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import { useLocalization, type Translate } from "../localization/Localization";
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
  const { t } = useLocalization();
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
        title={t("knowledge.unavailable")}
        detail={t("knowledge.unavailableDetail")}
      />
    );
  }
  if (query.isPending) return <ResourceState title={t("knowledge.loading")} />;
  if (query.error) {
    return (
      <ResourceState
        title={t("knowledge.loadFailed")}
        detail={messageOf(query.error, t("knowledge.operationFailed"))}
        action={t("resource.tryAgain")}
        onAction={() => void query.refetch()}
      />
    );
  }
  if (!query.data || query.data.data.length === 0) {
    return (
      <ResourceState
        title={t("knowledge.noScopes")}
        detail={t("knowledge.noScopesDetail")}
      />
    );
  }

  return (
    <div
      className="knowledge-document-list"
      aria-label={t("knowledge.documents")}
    >
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
  const { formatDateTime, t } = useLocalization();
  const queryClient = useQueryClient();
  const controller = useRef<AbortController | undefined>(undefined);
  const saving = useRef(false);
  const [draft, setDraft] = useState<KnowledgeDraft>(() =>
    openDraft(props.entry),
  );
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
      queryClient.setQueryData<Page<KnowledgeEntry>>(
        props.queryKey,
        (current) =>
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
      let message = messageOf(cause, t("knowledge.operationFailed"));
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
            message = t("knowledge.externalChange");
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
          <h4>{knowledgePath(props.entry.scope, t)}</h4>
          <p>{knowledgeScope(props.entry.scope, t)}</p>
        </div>
        {draft.updatedAt ? (
          <time dateTime={draft.updatedAt}>
            {formatKnowledgeTime(draft.updatedAt, formatDateTime)}
          </time>
        ) : (
          <span>{t("knowledge.notCreated")}</span>
        )}
      </header>
      <textarea
        value={draft.value}
        rows={10}
        maxLength={1_048_576}
        spellCheck={false}
        aria-label={t("knowledge.editorLabel", {
          scope: knowledgeScope(props.entry.scope, t),
        })}
        onChange={(event) => {
          setError(undefined);
          setDraft((current) => ({
            ...current,
            value: event.currentTarget.value,
          }));
        }}
      />
      <footer>
        <span>{dirty ? t("knowledge.unsaved") : t("knowledge.saved")}</span>
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => {
            setError(undefined);
            setDraft(openDraft(props.entry));
          }}
        >
          {t("knowledge.revert")}
        </button>
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => void save()}
        >
          {pending ? t("knowledge.saving") : t("knowledge.save")}
        </button>
      </footer>
      {error ? (
        <p className="knowledge-error" role="alert">
          {error}
        </p>
      ) : null}
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

function knowledgePath(scope: string, t: Translate) {
  if (scope === "home") return "~/.lyra/LYRA.md";
  if (scope === "projectRoot") return t("knowledge.path.projectRoot");
  return t("knowledge.path.workspace");
}

function knowledgeScope(scope: string, t: Translate) {
  if (scope === "home") return t("knowledge.scope.home");
  if (scope === "projectRoot") return t("knowledge.scope.projectRoot");
  return t("knowledge.scope.workspace");
}

function formatKnowledgeTime(
  value: string,
  formatDateTime: (value: Date, options?: Intl.DateTimeFormatOptions) => string,
) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? value
    : formatDateTime(date, {
        dateStyle: "medium",
        timeStyle: "short",
      });
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
