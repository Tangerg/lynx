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
  RuntimeConnection,
  Session,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  listWorkspaceFiles,
  readWorkspaceFile,
  runtimeQueryKeys,
  searchWorkspaceFiles,
} from "../../runtime/runtimeQueries";
import {
  maxCollapsedReviewFiles,
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
import { WorkspaceReview } from "./WorkspaceReview";

const fileWindowLines = 1_000;

interface ContextDockProps {
  connection: RuntimeConnection;
  session?: Session;
  onExpandedChange(expanded: boolean): void;
  children: ReactNode;
}

export function ContextDock(props: ContextDockProps) {
  const [states, setStates] = useState<Record<string, SessionDockState>>(
    readDockState,
  );
  const sessionId = props.session?.id;
  const workspacePath = props.session?.workspace.ref.path;
  const state = useMemo(() => {
    if (sessionId === undefined || workspacePath === undefined) return undefined;
    const stored = states[sessionId];
    return stored?.workspacePath === workspacePath
      ? stored
      : newDockState(workspacePath);
  }, [sessionId, states, workspacePath]);

  useEffect(() => writeDockState(states), [states]);
  useEffect(() => {
    props.onExpandedChange(
      state?.pane === "workspace" &&
        (state.workspaceView === "review" || state.selectedPath !== undefined),
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

  const setPane = (pane: DockPane) => update((current) => ({ ...current, pane }));
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
      <nav className="context-dock-tabs" aria-label="Context Dock sections">
        <button
          type="button"
          role="tab"
          aria-selected={state?.pane === "workspace"}
          disabled={state === undefined}
          onClick={() => setPane("workspace")}
        >
          Workspace
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={state?.pane !== "workspace"}
          onClick={() => setPane("session")}
        >
          Session
        </button>
      </nav>
      {props.session && state?.pane === "workspace" ? (
        props.session.workspace.availability === "available" ? (
          <WorkspaceBrowser
            connection={props.connection}
            workspace={props.session.workspace.ref}
            state={state}
            update={update}
            onOpenFile={openFile}
            onCloseFile={closeFile}
          />
        ) : (
          <DockState
            title="Workspace unavailable"
            detail="Reconnect this Session to an available directory before browsing files."
          />
        )
      ) : (
        <div className="session-context">{props.children}</div>
      )}
    </>
  );
}

interface WorkspaceBrowserProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  state: SessionDockState;
  update(change: (current: SessionDockState) => SessionDockState): void;
  onOpenFile(path: string, line?: number): void;
  onCloseFile(path: string): void;
}

function WorkspaceBrowser(props: WorkspaceBrowserProps) {
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
      aria-label="Workspace context"
    >
      <header className="workspace-browser-header">
        <div>
          <nav className="workspace-view-switch" aria-label="Workspace views">
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
              Files
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
              Review
            </button>
          </nav>
          <small title={props.workspace.path}>
            {compactPath(props.workspace.path)}
          </small>
        </div>
        {props.state.workspaceView === "files" ? (
          <form role="search" onSubmit={submitSearch}>
            <label className="sr-only" htmlFor="workspace-file-search">
              Search workspace text
            </label>
            <input
              id="workspace-file-search"
              type="search"
              value={props.state.searchDraft}
              placeholder="Search text"
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
      ) : (
        <div className="workspace-browser-body">
          <aside className="workspace-tree" aria-label="Workspace file tree">
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
  if (query.isPending) return <TreeNotice label="Loading files…" />;
  if (query.isError) return <TreeNotice label={messageOf(query.error)} error />;
  if (entries.length === 0) return <TreeNotice label="This folder is empty." />;
  return (
    <div className="directory-contents">
      {entries.map((entry) => (
        <FileTreeEntry
          key={entry.path}
          {...props}
          entry={entry}
        />
      ))}
      {query.hasNextPage ? (
        <button
          className="tree-more"
          type="button"
          disabled={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          {query.isFetchingNextPage ? "Loading…" : "Load more"}
        </button>
      ) : null}
    </div>
  );
}

function FileTreeEntry(props: DirectoryContentsProps & { entry: FileEntry }) {
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
        {props.entry.type === "symlink" ? <small>link</small> : null}
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
  if (result.isPending) return <TreeNotice label="Searching…" />;
  if (result.isError) return <TreeNotice label={messageOf(result.error)} error />;
  if (!result.data || result.data.matches.length === 0) {
    return <TreeNotice label="No text matches." />;
  }
  return (
    <div className="workspace-search-results">
      <header>
        <strong>{result.data.total.toLocaleString()} matches</strong>
        {result.data.total > result.data.matches.length ? (
          <small>first 200</small>
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
  return (
    <button
      className="workspace-search-hit"
      type="button"
      title={`${props.match.path}:${props.match.lineNumber}`}
      onClick={() => props.onOpen(props.match.path, props.match.lineNumber)}
    >
      <strong>{baseName(props.match.path)}</strong>
      <small>{props.match.path}:{props.match.lineNumber}</small>
      <span>{props.match.text.trim() || "Empty line"}</span>
    </button>
  );
}

function OpenFileTabs(props: {
  paths: string[];
  selectedPath?: string;
  onSelect(path: string): void;
  onClose(path: string): void;
}) {
  return (
    <div className="workspace-open-tabs" role="tablist" aria-label="Open files">
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
            aria-label={`Close ${path}`}
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
    return <DockState title="Opening file" detail={props.path} />;
  }
  if (file.isError) {
    return (
      <DockState
        title="File could not be opened"
        detail={messageOf(file.error)}
      />
    );
  }
  const lines = file.data.totalLines === 0 ? [] : file.data.content.split("\n");
  const servedStart = file.data.startLine || startLine;
  const servedEnd =
    file.data.endLine || (lines.length === 0 ? 0 : servedStart + lines.length - 1);
  return (
    <article className="workspace-file-reader" aria-label={props.path}>
      <header>
        <div>
          <strong>{baseName(props.path)}</strong>
          <small title={props.path}>{props.path}</small>
        </div>
        <nav className="file-window-actions" aria-label="File line range">
          <button
            type="button"
            disabled={servedStart <= 1}
            aria-label="Previous lines"
            onClick={() =>
              setStartLine(Math.max(1, startLine - fileWindowLines))
            }
          >
            ←
          </button>
          <span>
            {file.data.totalLines === 0
              ? "Empty file"
              : `${servedStart.toLocaleString()}–${servedEnd.toLocaleString()} / ${file.data.totalLines.toLocaleString()}`}
          </span>
          <button
            type="button"
            disabled={servedEnd >= file.data.totalLines}
            aria-label="Next lines"
            onClick={() => setStartLine(startLine + fileWindowLines)}
          >
            →
          </button>
        </nav>
      </header>
      {file.data.truncated ? (
        <p className="file-truncated" role="status">
          This view is bounded to an exact line window.
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

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "The workspace request failed.";
}
