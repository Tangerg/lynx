import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type Ref,
} from "react";

import type { Session } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import {
  compactPath,
  formatUpdatedAt,
  sessionStatus,
  workspaceName,
} from "./sessionPresentation";
import { ariaKeyShortcuts, commandByID } from "../shell/commandCatalog";
import { useActionMenu } from "../shell/useActionMenu";

interface SessionIndexProps {
  sessions: Session[];
  selectedId: string | undefined;
  pending: boolean;
  error: unknown;
  actionPending: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  onSelect: (sessionId: string) => void;
  onUpdate: (
    session: Session,
    patch: { title?: string; favorite?: boolean },
  ) => Promise<Session>;
  onRemove: (session: Session) => Promise<unknown>;
  onFork: (session: Session) => Promise<unknown>;
  onExport: (session: Session, format: "json" | "md") => Promise<unknown>;
  onRetry: () => void;
  onLoadMore: () => void;
  searchInputRef?: Ref<HTMLInputElement>;
}

export function SessionIndex(props: SessionIndexProps) {
  const { t } = useLocalization();
  const [search, setSearch] = useState("");
  const groups = useMemo(
    () => groupSessions(props.sessions, search),
    [props.sessions, search],
  );
  const visible = useMemo(
    () => groups.flatMap((group) => group.sessions),
    [groups],
  );

  if (props.pending) {
    return (
      <p className="panel-note" aria-busy="true">
        {t("session.loading")}
      </p>
    );
  }
  if (props.error && props.sessions.length === 0) {
    return (
      <div className="panel-error" role="alert">
        <p>{messageOf(props.error, t("session.changeFailed"))}</p>
        <button className="quiet-action" type="button" onClick={props.onRetry}>
          {t("session.retry")}
        </button>
      </div>
    );
  }
  if (props.sessions.length === 0) {
    return <p className="panel-note">{t("session.empty")}</p>;
  }

  return (
    <div className="session-index">
      <label className="session-search">
        <span aria-hidden="true">⌕</span>
        <span className="sr-only">{t("session.search")}</span>
        <input
          ref={props.searchInputRef}
          type="search"
          value={search}
          placeholder={t("session.search")}
          autoComplete="off"
          aria-keyshortcuts={ariaKeyShortcuts(
            commandByID("session.search").shortcut,
          )}
          onChange={(event) => setSearch(event.target.value)}
        />
        {search ? (
          <button
            type="button"
            aria-label={t("session.clearSearch")}
            onClick={() => setSearch("")}
          >
            ×
          </button>
        ) : null}
      </label>
      <span className="sr-only" aria-live="polite">
        {t(visible.length === 1 ? "session.shownOne" : "session.shownMany", {
          count: visible.length,
        })}
      </span>
      {props.error ? (
        <p className="session-refresh-warning" role="status">
          {t("session.refreshFailed", {
            detail: messageOf(props.error, t("session.changeFailed")),
          })}
        </p>
      ) : null}
      {visible.length === 0 ? (
        <p className="panel-note">
          {t("session.noMatch", { query: search.trim() })}
        </p>
      ) : (
        <nav
          className="session-list"
          aria-label={t("session.groupedLabel")}
          onKeyDown={(event) =>
            navigateSessions(event, visible, props.onSelect)
          }
        >
          {groups.map((group) => (
            <section className="session-group" key={group.path}>
              <header title={group.path}>
                <span>{workspaceName(group.path)}</span>
                <small>{group.sessions.length}</small>
              </header>
              {group.sessions.map((session) => (
                <SessionRow
                  key={session.id}
                  session={session}
                  selected={session.id === props.selectedId}
                  busy={props.actionPending}
                  onSelect={props.onSelect}
                  onUpdate={props.onUpdate}
                  onRemove={props.onRemove}
                  onFork={props.onFork}
                  onExport={props.onExport}
                />
              ))}
            </section>
          ))}
        </nav>
      )}
      {props.hasMore ? (
        <button
          className="load-more-sessions quiet-action"
          type="button"
          disabled={props.loadingMore}
          onClick={props.onLoadMore}
        >
          {props.loadingMore
            ? t("session.loadingOlder")
            : t("session.loadOlder")}
        </button>
      ) : null}
    </div>
  );
}

function SessionRow(props: {
  session: Session;
  selected: boolean;
  busy: boolean;
  onSelect: (sessionId: string) => void;
  onUpdate: SessionIndexProps["onUpdate"];
  onRemove: SessionIndexProps["onRemove"];
  onFork: SessionIndexProps["onFork"];
  onExport: SessionIndexProps["onExport"];
}) {
  const { formatDateTime, t } = useLocalization();
  const actionMenu = useActionMenu<
    HTMLDetailsElement,
    HTMLElement,
    HTMLDivElement
  >();
  const renameInput = useRef<HTMLInputElement>(null);
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState(props.session.title);
  const [renameSource, setRenameSource] = useState<Session>();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState<unknown>();

  useEffect(() => {
    if (!renaming) setDraft(props.session.title);
  }, [props.session.title, renaming]);
  useEffect(() => {
    if (renaming) renameInput.current?.select();
  }, [renaming]);
  useEffect(() => {
    if (actionMenu.open) return;
    setConfirmDelete(false);
    setError(undefined);
  }, [actionMenu.open]);

  const saveRename = async () => {
    const title = draft.trim();
    if (!title) {
      setError(new Error(t("session.titleRequired")));
      return;
    }
    const source = renameSource ?? props.session;
    if (title === source.title) {
      setRenaming(false);
      setRenameSource(undefined);
      return;
    }
    setError(undefined);
    try {
      await props.onUpdate(source, { title });
      setRenaming(false);
      setRenameSource(undefined);
    } catch (failure) {
      setError(failure);
    }
  };
  const toggleFavorite = async () => {
    setError(undefined);
    try {
      await props.onUpdate(props.session, {
        favorite: !props.session.favorite,
      });
      actionMenu.close({ restoreFocus: true });
    } catch (failure) {
      setError(failure);
    }
  };
  const remove = async () => {
    if (!confirmDelete) {
      setConfirmDelete(true);
      return;
    }
    setError(undefined);
    try {
      await props.onRemove(props.session);
    } catch (failure) {
      setError(failure);
    }
  };
  const fork = async () => {
    setError(undefined);
    try {
      await props.onFork(props.session);
      actionMenu.close();
    } catch (failure) {
      setError(failure);
    }
  };
  const exportAs = async (format: "json" | "md") => {
    setError(undefined);
    try {
      await props.onExport(props.session, format);
      actionMenu.close({ restoreFocus: true });
    } catch (failure) {
      setError(failure);
    }
  };

  return (
    <article className="session-row" data-selected={props.selected}>
      {renaming ? (
        <form
          className="session-rename-form"
          onSubmit={(event) => {
            event.preventDefault();
            void saveRename();
          }}
        >
          <label>
            <span className="sr-only">{t("session.title")}</span>
            <input
              ref={renameInput}
              value={draft}
              disabled={props.busy}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (
                  event.nativeEvent.isComposing ||
                  event.nativeEvent.keyCode === 229
                ) {
                  return;
                }
                if (event.key === "Escape") {
                  setDraft(props.session.title);
                  setRenaming(false);
                  setRenameSource(undefined);
                  setError(undefined);
                }
              }}
            />
          </label>
          <div>
            <button
              type="submit"
              disabled={props.busy}
              aria-label={t("session.saveTitle")}
            >
              ✓
            </button>
            <button
              type="button"
              disabled={props.busy}
              aria-label={t("session.cancelRename")}
              onClick={() => {
                setDraft(props.session.title);
                setRenaming(false);
                setRenameSource(undefined);
                setError(undefined);
              }}
            >
              ×
            </button>
          </div>
        </form>
      ) : (
        <button
          className="session-row-select"
          id={sessionControlId(props.session.id)}
          data-session-id={props.session.id}
          type="button"
          onClick={() => props.onSelect(props.session.id)}
        >
          <span className="session-row-main">
            <strong>
              {props.session.favorite ? (
                <span aria-label={t("session.favorite")}>★</span>
              ) : null}
              {props.session.title || t("session.untitled")}
            </strong>
            <small title={props.session.workspace.ref.path}>
              {compactPath(props.session.workspace.ref.path)}
            </small>
          </span>
          <span className="session-row-meta">
            <span className="session-state" data-status={props.session.status}>
              {sessionStatus(props.session.status, t)}
            </span>
            <time dateTime={props.session.updatedAt}>
              {formatUpdatedAt(props.session.updatedAt, formatDateTime)}
            </time>
          </span>
        </button>
      )}
      {!renaming ? (
        <details
          className="session-actions"
          ref={actionMenu.rootRef}
          open={actionMenu.open}
          onToggle={(event) => {
            actionMenu.setOpen(event.currentTarget.open);
          }}
        >
          <summary
            ref={actionMenu.triggerRef}
            aria-haspopup="menu"
            aria-expanded={actionMenu.open}
            aria-label={t("session.actionsFor", {
              title: props.session.title || t("session.untitled"),
            })}
          >
            •••
          </summary>
          <div ref={actionMenu.menuRef} role="menu">
            <button
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => {
                setDraft(props.session.title);
                setRenameSource(props.session);
                setRenaming(true);
                actionMenu.close();
              }}
            >
              {t("session.rename")}
            </button>
            <button
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => void toggleFavorite()}
            >
              {props.session.favorite
                ? t("session.removeFavorite")
                : t("session.favorite")}
            </button>
            <button
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => void fork()}
            >
              {t("session.fork")}
            </button>
            <button
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => void exportAs("json")}
            >
              {t("session.exportJSON")}
            </button>
            <button
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => void exportAs("md")}
            >
              {t("session.exportMarkdown")}
            </button>
            <button
              className={confirmDelete ? "confirm-delete" : undefined}
              type="button"
              role="menuitem"
              disabled={props.busy}
              onClick={() => void remove()}
            >
              {confirmDelete ? t("session.confirmDelete") : t("session.delete")}
            </button>
            {error ? (
              <p role="alert">{messageOf(error, t("session.changeFailed"))}</p>
            ) : null}
          </div>
        </details>
      ) : null}
      {renaming && error ? (
        <p className="session-action-error" role="alert">
          {messageOf(error, t("session.changeFailed"))}
        </p>
      ) : null}
    </article>
  );
}

interface SessionGroup {
  path: string;
  sessions: Session[];
}

function groupSessions(sessions: Session[], search: string): SessionGroup[] {
  const needle = search.trim().toLocaleLowerCase();
  const groups = new Map<string, Session[]>();
  for (const session of sessions) {
    const path = session.workspace.ref.path;
    if (
      needle &&
      !session.title.toLocaleLowerCase().includes(needle) &&
      !path.toLocaleLowerCase().includes(needle)
    ) {
      continue;
    }
    const group = groups.get(path);
    if (group) group.push(session);
    else groups.set(path, [session]);
  }
  return Array.from(groups, ([path, groupedSessions]) => ({
    path,
    sessions: groupedSessions,
  }));
}

function navigateSessions(
  event: ReactKeyboardEvent<HTMLElement>,
  sessions: Session[],
  onSelect: (sessionId: string) => void,
) {
  const control = (event.target as HTMLElement).closest<HTMLButtonElement>(
    ".session-row-select",
  );
  if (!control) return;
  const current = sessions.findIndex(
    (session) => session.id === control.dataset.sessionId,
  );
  let next = current;
  if (event.key === "ArrowDown")
    next = Math.min(current + 1, sessions.length - 1);
  else if (event.key === "ArrowUp") next = Math.max(current - 1, 0);
  else if (event.key === "Home") next = 0;
  else if (event.key === "End") next = sessions.length - 1;
  else return;
  event.preventDefault();
  const session = sessions[next];
  if (!session) return;
  onSelect(session.id);
  document.getElementById(sessionControlId(session.id))?.focus();
}

function sessionControlId(sessionId: string): string {
  return `session-${sessionId}`;
}

function messageOf(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}
