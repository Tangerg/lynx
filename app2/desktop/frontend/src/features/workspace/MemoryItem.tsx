import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type {
  AgentMemoryItem,
  AgentMemoryList,
  AgentMemoryReviewDecision,
  RuntimeConnection,
} from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import {
  deleteAgentMemory,
  reviewAgentMemory,
  runtimeQueryKeys,
  updateAgentMemory,
} from "../../runtime/runtimeQueries";
import {
  ActionError,
  MemoryByteCount,
  maxMemoryBytes,
  memoryBytes,
  useMemoryAction,
} from "./memoryWorkspaceAction";

interface MemoryItemProps {
  connection: RuntimeConnection;
  item: AgentMemoryItem;
  queryKey: QueryKey;
  onRefresh(): Promise<AgentMemoryList | undefined>;
}

export function PendingMemory(props: MemoryItemProps) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const action = useMemoryAction();

  const review = async (decision: AgentMemoryReviewDecision) => {
    const settled = await action.run(async (signal) => {
      try {
        await reviewAgentMemory(
          props.connection,
          props.item.id,
          decision,
          signal,
        );
      } catch (cause) {
        if (signal.aborted) throw cause;
        let latest: AgentMemoryList | undefined;
        try {
          latest = await props.onRefresh();
        } catch {
          throw cause;
        }
        const item = latest?.items.find((value) => value.id === props.item.id);
        const converged =
          decision === "approve"
            ? item?.status === "active"
            : item === undefined;
        if (!converged) throw cause;
      }
      return true;
    });
    if (!settled) return;
    queryClient.setQueryData<AgentMemoryList>(props.queryKey, (current) =>
      current === undefined
        ? current
        : {
            ...current,
            items: current.items.filter((item) => item.id !== props.item.id),
          },
    );
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.memory(props.connection),
    });
  };

  return (
    <article className="memory-item memory-item-pending">
      <p>{props.item.content}</p>
      <MemoryMeta item={props.item} />
      <footer>
        <button
          type="button"
          disabled={action.pending}
          onClick={() => void review("reject")}
        >
          {t("memory.reject")}
        </button>
        <button
          type="button"
          disabled={action.pending}
          onClick={() => void review("approve")}
        >
          {action.pending ? t("memory.reviewing") : t("memory.approve")}
        </button>
      </footer>
      <ActionError value={action.error} />
    </article>
  );
}

export function ActiveMemory(props: MemoryItemProps) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const action = useMemoryAction();
  const [editing, setEditing] = useState(false);
  const [editingVersion, setEditingVersion] = useState("");
  const [draft, setDraft] = useState(props.item.content);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const content = draft.trim();
  const contentBytes = memoryBytes(content);
  const stale = editing && editingVersion !== props.item.updatedAt;
  const dirty =
    content !== "" &&
    contentBytes <= maxMemoryBytes &&
    content !== props.item.content &&
    !stale;

  useEffect(() => {
    if (!editing) setDraft(props.item.content);
  }, [editing, props.item.content]);

  const commit = (saved: AgentMemoryItem) => {
    queryClient.setQueryData<AgentMemoryList>(props.queryKey, (current) =>
      current === undefined
        ? current
        : {
            ...current,
            items: current.items.map((item) =>
              item.id === saved.id ? saved : item,
            ),
          },
    );
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.memory(props.connection),
    });
  };

  const update = async (request: { content?: string; pinned?: boolean }) => {
    const saved = await action.run(async (signal) => {
      try {
        return await updateAgentMemory(
          props.connection,
          { id: props.item.id, ...request },
          signal,
        );
      } catch (cause) {
        if (signal.aborted) throw cause;
        let latest: AgentMemoryList | undefined;
        try {
          latest = await props.onRefresh();
        } catch {
          throw cause;
        }
        const item = latest?.items.find((value) => value.id === props.item.id);
        if (
          !item ||
          (request.content !== undefined && item.content !== request.content) ||
          (request.pinned !== undefined && item.pinned !== request.pinned)
        ) {
          throw cause;
        }
        return item;
      }
    });
    if (saved) commit(saved);
    return saved;
  };

  const remove = async () => {
    const removed = await action.run(async (signal) => {
      try {
        await deleteAgentMemory(props.connection, props.item.id, signal);
      } catch (cause) {
        if (signal.aborted) throw cause;
        let latest: AgentMemoryList | undefined;
        try {
          latest = await props.onRefresh();
        } catch {
          throw cause;
        }
        if (latest?.items.some((item) => item.id === props.item.id) !== false) {
          throw cause;
        }
      }
      return true;
    });
    if (!removed) return;
    queryClient.setQueryData<AgentMemoryList>(props.queryKey, (current) =>
      current === undefined
        ? current
        : {
            ...current,
            items: current.items.filter((item) => item.id !== props.item.id),
          },
    );
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.memory(props.connection),
    });
  };

  return (
    <article className="memory-item">
      {editing ? (
        <textarea
          rows={4}
          value={draft}
          spellCheck={false}
          aria-label={t("memory.editLabel")}
          onChange={(event) => {
            action.clearError();
            setDraft(event.currentTarget.value);
          }}
        />
      ) : (
        <p>{props.item.content}</p>
      )}
      <MemoryMeta item={props.item} />
      {stale ? (
        <p className="memory-error" role="alert">
          {t("memory.externalChange")}
        </p>
      ) : null}
      <footer>
        {editing ? (
          <>
            <MemoryByteCount value={contentBytes} />
            <button
              type="button"
              disabled={!dirty || action.pending}
              onClick={() => {
                void update({ content }).then((saved) => {
                  if (saved) setEditing(false);
                });
              }}
            >
              {action.pending ? t("memory.saving") : t("memory.save")}
            </button>
            <button
              type="button"
              disabled={action.pending}
              onClick={() => {
                setDraft(props.item.content);
                setEditing(false);
                setEditingVersion("");
                action.clearError();
              }}
            >
              {t("memory.cancel")}
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              disabled={action.pending}
              aria-pressed={props.item.pinned}
              onClick={() => void update({ pinned: !props.item.pinned })}
            >
              {props.item.pinned ? t("memory.unpin") : t("memory.pin")}
            </button>
            <button
              type="button"
              disabled={action.pending}
              onClick={() => {
                setConfirmDelete(false);
                setEditingVersion(props.item.updatedAt);
                setEditing(true);
              }}
            >
              {t("memory.edit")}
            </button>
            <button
              type="button"
              disabled={action.pending}
              className={confirmDelete ? "danger" : undefined}
              onClick={() => {
                if (!confirmDelete) {
                  setConfirmDelete(true);
                  return;
                }
                void remove();
              }}
            >
              {confirmDelete ? t("memory.confirmDelete") : t("memory.delete")}
            </button>
          </>
        )}
      </footer>
      <ActionError value={action.error} />
    </article>
  );
}

function MemoryMeta(props: { item: AgentMemoryItem }) {
  const { formatDateTime, t } = useLocalization();
  const updatedAt = new Date(props.item.updatedAt);
  return (
    <div className="memory-meta">
      {props.item.pinned ? <strong>{t("memory.pinned")}</strong> : null}
      <span>
        {props.item.origin === "auto"
          ? t("memory.lyraProposal")
          : t("memory.userAuthored")}
      </span>
      {props.item.sessionId ? <span>{t("memory.fromSession")}</span> : null}
      <time dateTime={props.item.updatedAt}>
        {Number.isNaN(updatedAt.valueOf())
          ? props.item.updatedAt
          : formatDateTime(updatedAt, { dateStyle: "medium" })}
      </time>
    </div>
  );
}
