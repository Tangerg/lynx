import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type FormEvent,
  type ReactNode,
} from "react";

import type {
  FileEntry,
  GrepMatch,
  Item,
  PendingInterruptSet,
  RunRef,
  RuntimeConnection,
  Session,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import { SessionActivity } from "../agent/SessionActivity";
import type { LiveToolOutput } from "../agent/agentSessionTypes";
import {
  listWorkspaceFiles,
  readWorkspaceFile,
  runtimeQueryKeys,
  searchWorkspaceFiles,
} from "../../runtime/runtimeQueries";
import {
  maxCollapsedReviewFiles,
  maxCodebaseQueryLength,
  maxExpandedDirectories,
  maxOpenFiles,
  newDockState,
  readDockState,
  rememberDockState,
  writeDockState,
  type DiffLayout,
  type DockPane,
  type ReviewMode,
  type SessionDockState,
} from "./contextDockState";
import { CodebaseWorkspace } from "./CodebaseWorkspace";
import { ResourcesWorkspace } from "./ResourcesWorkspace";
import { WorkspaceReview } from "./WorkspaceReview";

const fileWindowLines = 1_000;

interface ContextDockProps {
  connection: RuntimeConnection;
  session?: Session;
  runs: RunRef[];
  items: Item[];
  interrupts: PendingInterruptSet[];
  liveToolOutputs: Record<string, LiveToolOutput>;
  actionPending: boolean;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  skillsEnabled: boolean;
  knowledgeEnabled: boolean;
  memoryEnabled: boolean;
  onExpandedChange(expanded: boolean): void;
  onCancelRun(runId: string): Promise<void>;
  children: ReactNode;
}

export function ContextDock(props: ContextDockProps) {
  const { t } = useLocalization();
  const [states, setStates] =
    useState<Record<string, SessionDockState>>(readDockState);
  const sessionId = props.session?.id;
  const workspacePath = props.session?.workspace.ref.path;
  const state = useMemo(() => {
    if (sessionId === undefined || workspacePath === undefined)
      return undefined;
    const stored = states[sessionId];
    return stored?.workspacePath === workspacePath
      ? stored
      : newDockState(workspacePath);
  }, [sessionId, states, workspacePath]);

  useEffect(() => writeDockState(states), [states]);
  useEffect(() => {
    props.onExpandedChange(
      state?.pane === "workspace" &&
        (state.workspaceView !== "files" || state.selectedPath !== undefined),
    );
  }, [
    props.onExpandedChange,
    state?.pane,
    state?.selectedPath,
    state?.workspaceView,
  ]);

  const update = useCallback(
    (change: (current: SessionDockState) => SessionDockState) => {
      if (sessionId === undefined || workspacePath === undefined) return;
      setStates((current) => {
        const source =
          current[sessionId]?.workspacePath === workspacePath
            ? current[sessionId]
            : newDockState(workspacePath);
        return rememberDockState(current, sessionId, {
          ...change(source),
          touchedAt: Date.now(),
        });
      });
    },
    [sessionId, workspacePath],
  );

  const setPane = (pane: DockPane) =>
    update((current) => ({ ...current, pane }));
  const openFile = (path: string, line?: number) =>
    update((current) => {
      const openPaths = [
        ...current.openPaths.filter((candidate) => candidate !== path),
        path,
      ].slice(-maxOpenFiles);
      return {
        ...current,
        pane: "workspace",
        workspaceView: "files",
        openPaths,
        selectedPath: path,
        targetLines:
          line === undefined
            ? current.targetLines
            : { ...current.targetLines, [path]: line },
      };
    });
  const closeFile = (path: string) =>
    update((current) => {
      const openPaths = current.openPaths.filter(
        (candidate) => candidate !== path,
      );
      const targetLines = { ...current.targetLines };
      delete targetLines[path];
      return {
        ...current,
        openPaths,
        selectedPath:
          current.selectedPath === path
            ? openPaths.at(-1)
            : current.selectedPath,
        targetLines,
      };
    });

  return (
    <>
      <nav
        className="context-dock-tabs"
        aria-label={t("workspace.dockSections")}
      >
        <button
          type="button"
          role="tab"
          aria-selected={state?.pane === "workspace"}
          disabled={state === undefined}
          onClick={() => setPane("workspace")}
        >
          {t("workspace.title")}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={state?.pane !== "workspace"}
          onClick={() => setPane("session")}
        >
          {t("workspace.session")}
        </button>
      </nav>
      {props.session && state?.pane === "workspace" ? (
        props.session.workspace.availability === "available" ? (
          <WorkspaceBrowser
            connection={props.connection}
            workspace={props.session.workspace.ref}
            state={state}
            skillsEnabled={props.skillsEnabled}
            knowledgeEnabled={props.knowledgeEnabled}
            memoryEnabled={props.memoryEnabled}
            update={update}
            onOpenFile={openFile}
            onCloseFile={closeFile}
          />
        ) : (
          <DockState
            title={t("workspace.unavailable")}
            detail={t("workspace.unavailableDetail")}
          />
        )
      ) : (
        <div className="session-context">
          {props.session && state ? (
            <SessionActivity
              view={state.sessionView}
              runs={props.runs}
              items={props.items}
              interrupts={props.interrupts}
              liveToolOutputs={props.liveToolOutputs}
              actionPending={props.actionPending}
              cancelingRunId={props.cancelingRunId}
              cancelError={props.cancelError}
              onViewChange={(sessionView) =>
                update((current) => ({ ...current, sessionView }))
              }
              onCancelRun={props.onCancelRun}
            >
              {props.children}
            </SessionActivity>
          ) : (
            props.children
          )}
        </div>
      )}
    </>
  );
}

interface WorkspaceBrowserProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  state: SessionDockState;
  skillsEnabled: boolean;
  knowledgeEnabled: boolean;
  memoryEnabled: boolean;
  update(change: (current: SessionDockState) => SessionDockState): void;
  onOpenFile(path: string, line?: number): void;
  onCloseFile(path: string): void;
}

function WorkspaceBrowser(props: WorkspaceBrowserProps) {
  const { t } = useLocalization();
  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    props.update((current) => ({
      ...current,
      searchQuery: current.searchDraft.trim(),
    }));
  };
  const toggleDirectory = (path: string) =>
    props.update((current) => ({
      ...current,
      expandedDirectories: current.expandedDirectories.includes(path)
        ? current.expandedDirectories.filter((candidate) => candidate !== path)
        : [...current.expandedDirectories, path].slice(-maxExpandedDirectories),
    }));

  return (
    <section
      className="workspace-browser"
      data-reading={props.state.selectedPath !== undefined}
      aria-label={t("workspace.context")}
    >
      <header className="workspace-browser-header">
        <div>
          <nav
            className="workspace-view-switch"
            aria-label={t("workspace.views")}
          >
            <button
              type="button"
              aria-current={
                props.state.workspaceView === "files" ? "page" : undefined
              }
              onClick={() =>
                props.update((current) => ({
                  ...current,
                  workspaceView: "files",
                }))
              }
            >
              {t("workspace.files")}
            </button>
            <button
              type="button"
              aria-current={
                props.state.workspaceView === "review" ? "page" : undefined
              }
              onClick={() =>
                props.update((current) => ({
                  ...current,
                  workspaceView: "review",
                }))
              }
            >
              {t("workspace.review")}
            </button>
            <button
              type="button"
              aria-current={
                props.state.workspaceView === "codebase" ? "page" : undefined
              }
              onClick={() =>
                props.update((current) => ({
                  ...current,
                  workspaceView: "codebase",
                }))
              }
            >
              {t("workspace.codebase")}
            </button>
            <button
              type="button"
              aria-current={
                props.state.workspaceView === "resources" ? "page" : undefined
              }
              onClick={() =>
                props.update((current) => ({
                  ...current,
                  workspaceView: "resources",
                }))
              }
            >
              {t("workspace.resources")}
            </button>
          </nav>
          <small title={props.workspace.path}>
            {compactPath(props.workspace.path)}
          </small>
        </div>
        {props.state.workspaceView === "files" ? (
          <form role="search" onSubmit={submitSearch}>
            <label className="sr-only" htmlFor="workspace-file-search">
              {t("workspace.searchText")}
            </label>
            <input
              id="workspace-file-search"
              type="search"
              value={props.state.searchDraft}
              placeholder={t("workspace.searchPlaceholder")}
              onChange={(event) => {
                const value = event.currentTarget.value;
                props.update((current) => ({
                  ...current,
                  searchDraft: value,
                  ...(value === "" ? { searchQuery: "" } : {}),
                }));
              }}
            />
          </form>
        ) : null}
      </header>
      {props.state.workspaceView === "files" &&
      props.state.openPaths.length > 0 ? (
        <OpenFileTabs
          paths={props.state.openPaths}
          selectedPath={props.state.selectedPath}
          onSelect={props.onOpenFile}
          onClose={props.onCloseFile}
        />
      ) : null}
      {props.state.workspaceView === "review" ? (
        <WorkspaceReview
          connection={props.connection}
          workspace={props.workspace}
          selectedPath={props.state.selectedChangePath}
          mode={props.state.reviewMode}
          layout={props.state.diffLayout}
          navigatorOpen={props.state.reviewNavigatorOpen}
          collapsedPaths={props.state.collapsedReviewPaths}
          onSelect={(path) =>
            props.update((current) => ({
              ...current,
              selectedChangePath: path,
            }))
          }
          onModeChange={(mode: ReviewMode) =>
            props.update((current) => ({
              ...current,
              reviewMode: mode,
              selectedChangePath: undefined,
            }))
          }
          onLayoutChange={(layout: DiffLayout) =>
            props.update((current) => ({ ...current, diffLayout: layout }))
          }
          onNavigatorOpenChange={(reviewNavigatorOpen) =>
            props.update((current) => ({
              ...current,
              reviewNavigatorOpen,
            }))
          }
          onToggleCollapsed={(path) =>
            props.update((current) => ({
              ...current,
              collapsedReviewPaths: current.collapsedReviewPaths.includes(path)
                ? current.collapsedReviewPaths.filter(
                    (candidate) => candidate !== path,
                  )
                : [...current.collapsedReviewPaths, path].slice(
                    -maxCollapsedReviewFiles,
                  ),
            }))
          }
          onOpenFile={props.onOpenFile}
        />
      ) : props.state.workspaceView === "resources" ? (
        <ResourcesWorkspace
          connection={props.connection}
          workspace={props.workspace}
          skillsEnabled={props.skillsEnabled}
          knowledgeEnabled={props.knowledgeEnabled}
          memoryEnabled={props.memoryEnabled}
          view={props.state.resourceView}
          skillView={props.state.skillView}
          onViewChange={(resourceView) =>
            props.update((current) => ({ ...current, resourceView }))
          }
          onSkillViewChange={(skillView) =>
            props.update((current) => ({ ...current, skillView }))
          }
        />
      ) : props.state.workspaceView === "codebase" ? (
        <CodebaseWorkspace
          connection={props.connection}
          workspace={props.workspace}
          draft={props.state.codebaseDraft}
          query={props.state.codebaseQuery}
          onDraftChange={(codebaseDraft) =>
            props.update((current) => ({
              ...current,
              codebaseDraft: codebaseDraft.slice(0, maxCodebaseQueryLength),
              ...(codebaseDraft === "" ? { codebaseQuery: "" } : {}),
            }))
          }
          onSubmit={(codebaseQuery) =>
            props.update((current) => ({ ...current, codebaseQuery }))
          }
          onOpenFile={props.onOpenFile}
        />
      ) : (
        <div className="workspace-browser-body">
          <aside
            className="workspace-tree"
            aria-label={t("workspace.fileTree")}
          >
            {props.state.searchQuery ? (
              <WorkspaceSearchResults
                connection={props.connection}
                workspace={props.workspace}
                query={props.state.searchQuery}
                onOpen={props.onOpenFile}
              />
            ) : (
              <DirectoryContents
                connection={props.connection}
                workspace={props.workspace}
                path=""
                depth={0}
                selectedPath={props.state.selectedPath}
                expandedDirectories={props.state.expandedDirectories}
                onToggle={toggleDirectory}
                onOpen={props.onOpenFile}
              />
            )}
          </aside>
          {props.state.selectedPath ? (
            <FileReader
              connection={props.connection}
              workspace={props.workspace}
              path={props.state.selectedPath}
              targetLine={props.state.targetLines[props.state.selectedPath]}
            />
          ) : null}
        </div>
      )}
    </section>
  );
}

interface DirectoryContentsProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  path: string;
  depth: number;
  selectedPath?: string;
  expandedDirectories: string[];
  onToggle(path: string): void;
  onOpen(path: string): void;
}

function DirectoryContents(props: DirectoryContentsProps) {
  const { t } = useLocalization();
  const query = useInfiniteQuery({
    queryKey: runtimeQueryKeys.workspaceFiles(
      props.connection,
      props.workspace.path,
      props.path,
    ),
    queryFn: ({ pageParam, signal }) =>
      listWorkspaceFiles(
        props.connection,
        props.workspace,
        props.path,
        pageParam,
        signal,
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: 2,
  });
  const entries = query.data?.pages.flatMap((page) => page.data) ?? [];
  if (query.isPending)
    return <TreeNotice label={t("workspace.loadingFiles")} />;
  if (query.isError) {
    return (
      <TreeNotice
        label={messageOf(query.error, t("workspace.requestFailed"))}
        error
      />
    );
  }
  if (entries.length === 0) {
    return <TreeNotice label={t("workspace.emptyFolder")} />;
  }
  return (
    <div className="directory-contents">
      {entries.map((entry) => (
        <FileTreeEntry key={entry.path} {...props} entry={entry} />
      ))}
      {query.hasNextPage ? (
        <button
          className="tree-more"
          type="button"
          disabled={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          {query.isFetchingNextPage
            ? t("workspace.loadingMore")
            : t("workspace.loadMore")}
        </button>
      ) : null}
    </div>
  );
}

function FileTreeEntry(props: DirectoryContentsProps & { entry: FileEntry }) {
  const { t } = useLocalization();
  const directory = props.entry.type === "dir";
  const expanded =
    directory && props.expandedDirectories.includes(props.entry.path);
  const style = { "--tree-depth": props.depth } as CSSProperties;
  return (
    <div className="file-tree-entry">
      <button
        type="button"
        style={style}
        data-kind={props.entry.type}
        data-selected={props.selectedPath === props.entry.path}
        title={props.entry.path}
        onClick={() =>
          directory
            ? props.onToggle(props.entry.path)
            : props.onOpen(props.entry.path)
        }
      >
        <span aria-hidden="true">
          {directory ? (expanded ? "▾" : "▸") : "·"}
        </span>
        <span>{props.entry.name}</span>
        {props.entry.type === "symlink" ? (
          <small>{t("workspace.symlink")}</small>
        ) : null}
      </button>
      {expanded ? (
        <DirectoryContents
          {...props}
          path={props.entry.path}
          depth={props.depth + 1}
        />
      ) : null}
    </div>
  );
}

function WorkspaceSearchResults(props: {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  query: string;
  onOpen(path: string, line: number): void;
}) {
  const { formatNumber, t } = useLocalization();
  const result = useQuery({
    queryKey: runtimeQueryKeys.workspaceSearch(
      props.connection,
      props.workspace.path,
      props.query,
    ),
    queryFn: ({ signal }) =>
      searchWorkspaceFiles(
        props.connection,
        props.workspace,
        props.query,
        signal,
      ),
    enabled: props.query !== "",
    retry: 1,
  });
  if (result.isPending) return <TreeNotice label={t("workspace.searching")} />;
  if (result.isError) {
    return (
      <TreeNotice
        label={messageOf(result.error, t("workspace.requestFailed"))}
        error
      />
    );
  }
  if (!result.data || result.data.matches.length === 0) {
    return <TreeNotice label={t("workspace.noMatches")} />;
  }
  return (
    <div className="workspace-search-results">
      <header>
        <strong>
          {t("workspace.matchCount", {
            count: formatNumber(result.data.total),
          })}
        </strong>
        {result.data.total > result.data.matches.length ? (
          <small>{t("workspace.firstResults", { count: 200 })}</small>
        ) : null}
      </header>
      {result.data.matches.map((match, index) => (
        <SearchHit
          key={`${match.path}:${match.lineNumber}:${index}`}
          match={match}
          onOpen={props.onOpen}
        />
      ))}
    </div>
  );
}

function SearchHit(props: {
  match: GrepMatch;
  onOpen(path: string, line: number): void;
}) {
  const { t } = useLocalization();
  return (
    <button
      className="workspace-search-hit"
      type="button"
      title={`${props.match.path}:${props.match.lineNumber}`}
      onClick={() => props.onOpen(props.match.path, props.match.lineNumber)}
    >
      <strong>{baseName(props.match.path)}</strong>
      <small dir="ltr">
        {props.match.path}:{props.match.lineNumber}
      </small>
      <span>{props.match.text.trim() || t("workspace.emptyLine")}</span>
    </button>
  );
}

function OpenFileTabs(props: {
  paths: string[];
  selectedPath?: string;
  onSelect(path: string): void;
  onClose(path: string): void;
}) {
  const { t } = useLocalization();
  return (
    <div
      className="workspace-open-tabs"
      role="tablist"
      aria-label={t("workspace.openFiles")}
    >
      {props.paths.map((path) => (
        <span key={path} data-selected={path === props.selectedPath}>
          <button
            type="button"
            role="tab"
            aria-selected={path === props.selectedPath}
            title={path}
            onClick={() => props.onSelect(path)}
          >
            {baseName(path)}
          </button>
          <button
            type="button"
            aria-label={t("workspace.closeFile", { path })}
            onClick={() => props.onClose(path)}
          >
            ×
          </button>
        </span>
      ))}
    </div>
  );
}

function FileReader(props: {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  path: string;
  targetLine?: number;
}) {
  const { direction, formatNumber, t } = useLocalization();
  const [startLine, setStartLine] = useState(() =>
    windowStart(props.targetLine),
  );
  useEffect(() => {
    setStartLine(windowStart(props.targetLine));
  }, [props.path, props.targetLine]);
  const endLine = startLine + fileWindowLines - 1;
  const file = useQuery({
    queryKey: runtimeQueryKeys.workspaceFile(
      props.connection,
      props.workspace.path,
      props.path,
      startLine,
    ),
    queryFn: ({ signal }) =>
      readWorkspaceFile(
        props.connection,
        props.workspace,
        props.path,
        startLine,
        endLine,
        signal,
      ),
    retry: 1,
  });
  useEffect(() => {
    if (file.data === undefined || props.targetLine === undefined) return;
    const frame = window.requestAnimationFrame(() => {
      document
        .getElementById(fileLineID(props.path, props.targetLine ?? 0))
        ?.scrollIntoView({ block: "center" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [file.data, props.path, props.targetLine]);
  if (file.isPending) {
    return <DockState title={t("workspace.openingFile")} detail={props.path} />;
  }
  if (file.isError) {
    return (
      <DockState
        title={t("workspace.fileOpenFailed")}
        detail={messageOf(file.error, t("workspace.requestFailed"))}
      />
    );
  }
  const lines = file.data.totalLines === 0 ? [] : file.data.content.split("\n");
  const servedStart = file.data.startLine || startLine;
  const servedEnd =
    file.data.endLine ||
    (lines.length === 0 ? 0 : servedStart + lines.length - 1);
  return (
    <article className="workspace-file-reader" aria-label={props.path}>
      <header>
        <div dir="ltr">
          <strong>{baseName(props.path)}</strong>
          <small title={props.path}>{props.path}</small>
        </div>
        <nav
          className="file-window-actions"
          aria-label={t("workspace.lineRange")}
        >
          <button
            type="button"
            disabled={servedStart <= 1}
            aria-label={t("workspace.previousLines")}
            onClick={() =>
              setStartLine(Math.max(1, startLine - fileWindowLines))
            }
          >
            {direction === "rtl" ? "→" : "←"}
          </button>
          <span>
            {file.data.totalLines === 0
              ? t("workspace.emptyFile")
              : t("workspace.linePosition", {
                  start: formatNumber(servedStart),
                  end: formatNumber(servedEnd),
                  total: formatNumber(file.data.totalLines),
                })}
          </span>
          <button
            type="button"
            disabled={servedEnd >= file.data.totalLines}
            aria-label={t("workspace.nextLines")}
            onClick={() => setStartLine(startLine + fileWindowLines)}
          >
            {direction === "rtl" ? "←" : "→"}
          </button>
        </nav>
      </header>
      {file.data.truncated ? (
        <p className="file-truncated" role="status">
          {t("workspace.boundedWindow")}
        </p>
      ) : null}
      <pre data-language={languageOf(props.path)}>
        <code>
          {lines.map((line, index) => {
            const lineNumber = servedStart + index;
            return (
              <span
                className="workspace-code-line"
                id={fileLineID(props.path, lineNumber)}
                data-target={props.targetLine === lineNumber}
                key={lineNumber}
              >
                <i>{lineNumber}</i>
                <b>{line || " "}</b>
              </span>
            );
          })}
        </code>
      </pre>
    </article>
  );
}

function TreeNotice(props: { label: string; error?: boolean }) {
  return (
    <p className="tree-notice" data-error={props.error}>
      {props.label}
    </p>
  );
}

function DockState(props: { title: string; detail: string }) {
  return (
    <section className="dock-state">
      <strong>{props.title}</strong>
      <p>{props.detail}</p>
    </section>
  );
}

function fileLineID(path: string, line: number) {
  return `workspace-line-${encodeURIComponent(path)}-${line}`;
}

function windowStart(targetLine: number | undefined) {
  if (targetLine === undefined || targetLine <= 1) return 1;
  return Math.floor((targetLine - 1) / fileWindowLines) * fileWindowLines + 1;
}

function languageOf(path: string) {
  const extension = path.split(".").at(-1)?.toLocaleLowerCase();
  return extension && extension !== path ? extension : "text";
}

function baseName(path: string) {
  return path.split("/").at(-1) || path;
}

function compactPath(path: string) {
  const parts = path.split("/").filter(Boolean);
  return parts.length <= 2 ? path : `…/${parts.slice(-2).join("/")}`;
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
