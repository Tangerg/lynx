import {
  useQuery,
  useQueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";

import type {
  AgentMemoryList,
  AgentMemoryScope,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  useLocalization,
  type Translate,
} from "../localization/Localization";
import {
  addAgentMemory,
  listAgentMemory,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import { ActiveMemory, PendingMemory } from "./MemoryItem";
import {
  ActionError,
  MemoryByteCount,
  maxMemoryBytes,
  memoryBytes,
  messageOf,
  useMemoryAction,
} from "./memoryWorkspaceAction";
import { ResourceState } from "./ResourceState";

interface MemoryWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  enabled: boolean;
}

export function MemoryWorkspace(props: MemoryWorkspaceProps) {
  const { formatNumber, t } = useLocalization();
  const [scope, setScope] = useState<AgentMemoryScope>("project");
  const queryKey = runtimeQueryKeys.memoryTarget(
    props.connection,
    scope,
    props.workspace.path,
  );
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) =>
      listAgentMemory(props.connection, scope, props.workspace, signal),
    enabled: props.enabled,
    retry: 2,
  });

  if (!props.enabled) {
    return (
      <ResourceState
        title={t("memory.unavailable")}
        detail={t("memory.unavailableDetail")}
      />
    );
  }

  const refresh = async () => (await query.refetch()).data;
  const items = query.data?.items ?? [];
  const pending = items.filter((item) => item.status === "pending");
  const active = items.filter((item) => item.status === "active");

  return (
    <div className="memory-workspace">
      <header className="memory-toolbar">
        <div className="memory-scope-switch" aria-label={t("memory.scope")}>
          <ScopeButton
            label={t("memory.project")}
            selected={scope === "project"}
            onSelect={() => setScope("project")}
          />
          <ScopeButton
            label={t("memory.user")}
            selected={scope === "user"}
            onSelect={() => setScope("user")}
          />
        </div>
        <span>
          {t("memory.counts", {
            pending: formatNumber(pending.length),
            active: formatNumber(active.length),
          })}
        </span>
      </header>
      <AddMemory
        connection={props.connection}
        workspace={props.workspace}
        scope={scope}
        queryKey={queryKey}
      />
      {query.isPending ? (
        <ResourceState title={t("memory.loading")} />
      ) : query.error ? (
        <ResourceState
          title={t("memory.loadFailed")}
          detail={messageOf(query.error, t("memory.operationFailed"))}
          action={t("resource.tryAgain")}
          onAction={() => void query.refetch()}
        />
      ) : items.length === 0 ? (
        <ResourceState
          title={
            scope === "project"
              ? t("memory.emptyProject")
              : t("memory.emptyUser")
          }
          detail={t("memory.emptyDetail")}
        />
      ) : (
        <div className="memory-list">
          {pending.length > 0 ? (
            <MemorySection label={t("memory.pendingReview")}>
              {pending.map((item) => (
                <PendingMemory
                  key={item.id}
                  connection={props.connection}
                  item={item}
                  queryKey={queryKey}
                  onRefresh={refresh}
                />
              ))}
            </MemorySection>
          ) : null}
          {active.length > 0 ? (
            <MemorySection label={t("memory.active")}>
              {active.map((item) => (
                <ActiveMemory
                  key={item.id}
                  connection={props.connection}
                  item={item}
                  queryKey={queryKey}
                  onRefresh={refresh}
                />
              ))}
            </MemorySection>
          ) : null}
        </div>
      )}
    </div>
  );
}

function ScopeButton(props: {
  label: string;
  selected: boolean;
  onSelect(): void;
}) {
  const { t } = useLocalization();
  return (
    <button
      type="button"
      aria-pressed={props.selected}
      onClick={props.onSelect}
    >
      {props.label}
    </button>
  );
}

function AddMemory(props: {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  scope: AgentMemoryScope;
  queryKey: QueryKey;
}) {
  const queryClient = useQueryClient();
  const action = useMemoryAction();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const content = draft.trim();
  const contentBytes = memoryBytes(content);
  const contentValid = content !== "" && contentBytes <= maxMemoryBytes;

  useEffect(() => {
    setOpen(false);
    setDraft("");
    action.clearError();
  }, [action.clearError, props.scope]);

  if (!open) {
    return (
      <div className="memory-add-collapsed">
        <button type="button" onClick={() => setOpen(true)}>
          {t("memory.add")}
        </button>
      </div>
    );
  }

  const submit = async () => {
    if (!contentValid) return;
    const saved = await action.run((signal) =>
      addAgentMemory(
        props.connection,
        props.scope,
        props.workspace,
        content,
        signal,
      ),
    );
    if (!saved) return;
    queryClient.setQueryData<AgentMemoryList>(props.queryKey, (current) =>
      current === undefined
        ? current
        : {
            ...current,
            items: [
              saved,
              ...current.items.filter((item) => item.id !== saved.id),
            ],
          },
    );
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.memory(props.connection),
    });
    setDraft("");
    setOpen(false);
  };

  return (
    <div className="memory-add-form">
      <textarea
        rows={3}
        value={draft}
        spellCheck={false}
        aria-label={t("memory.addLabel", {
          scope: memoryScopeLabel(props.scope, t),
        })}
        placeholder={t("memory.placeholder")}
        onChange={(event) => {
          action.clearError();
          setDraft(event.currentTarget.value);
        }}
      />
      <footer>
        <MemoryByteCount value={contentBytes} />
        <button
          type="button"
          disabled={!contentValid || action.pending}
          onClick={() => void submit()}
        >
          {action.pending ? t("memory.saving") : t("memory.save")}
        </button>
        <button
          type="button"
          disabled={action.pending}
          onClick={() => {
            setDraft("");
            setOpen(false);
            action.clearError();
          }}
        >
          {t("memory.cancel")}
        </button>
      </footer>
      <ActionError value={action.error} />
    </div>
  );
}

function MemorySection(props: { label: string; children: ReactNode }) {
  return (
    <section className="memory-section">
      <h4>{props.label}</h4>
      <div>{props.children}</div>
    </section>
  );
}

function memoryScopeLabel(scope: AgentMemoryScope, t: Translate) {
  return scope === "project" ? t("memory.project") : t("memory.user");
}
