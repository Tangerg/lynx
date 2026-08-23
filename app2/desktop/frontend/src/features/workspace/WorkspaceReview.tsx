import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, type ReactNode } from "react";

import type {
  DiffRow,
  FileDiff,
  RuntimeConnection,
  WorkspaceFileChange,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  getWorkspaceDiff,
  listWorkspaceChanges,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import type { DiffLayout, ReviewMode } from "./contextDockState";

interface WorkspaceReviewProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  selectedPath: string | undefined;
  mode: ReviewMode;
  layout: DiffLayout;
  navigatorOpen: boolean;
  collapsedPaths: string[];
  onSelect(path: string): void;
  onModeChange(mode: ReviewMode): void;
  onLayoutChange(layout: DiffLayout): void;
  onNavigatorOpenChange(open: boolean): void;
  onToggleCollapsed(path: string): void;
  onOpenFile(path: string): void;
}

export function WorkspaceReview(props: WorkspaceReviewProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const changes = useQuery({
    queryKey: runtimeQueryKeys.workspaceChanges(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listWorkspaceChanges(props.connection, props.workspace, signal),
    enabled: props.mode === "worktree",
    retry: 1,
  });
  const diff = useQuery({
    queryKey: runtimeQueryKeys.workspaceDiff(
      props.connection,
      props.workspace.path,
      "",
      props.mode,
    ),
    queryFn: ({ signal }) =>
      getWorkspaceDiff(
        props.connection,
        props.workspace,
        "",
        props.mode,
        signal,
      ),
    retry: 1,
  });
  const files = diff.data?.files ?? [];
  const selectedPath = files.some((file) => file.path === props.selectedPath)
    ? props.selectedPath
    : files[0]?.path;

  useEffect(() => {
    if (selectedPath !== undefined && selectedPath !== props.selectedPath) {
      props.onSelect(selectedPath);
    }
  }, [props.onSelect, props.selectedPath, selectedPath]);
  useEffect(() => {
    if (selectedPath === undefined || diff.data === undefined) return;
    const selector = `[data-diff-file="${CSS.escape(selectedPath)}"]`;
    scrollRef.current
      ?.querySelector(selector)
      ?.scrollIntoView({ block: "start" });
  }, [diff.data, selectedPath]);

  const worktreeError = props.mode === "worktree" && changes.isError;
  if (diff.isPending || (props.mode === "worktree" && changes.isPending)) {
    return <ReviewState title="Building workspace review" />;
  }
  if (diff.isError || worktreeError) {
    const error = diff.isError ? diff.error : changes.error;
    return (
      <ReviewState
        title={
          props.mode === "base"
            ? "Branch comparison is unavailable"
            : "Workspace review could not be loaded"
        }
        detail={messageOf(error)}
        onRetry={() => {
          void diff.refetch();
          if (props.mode === "worktree") void changes.refetch();
        }}
      />
    );
  }
  if (diff.data.truncated && files.length === 0) {
    return (
      <ReviewState
        title="Review boundary reached"
        detail="The first changed file exceeds the 100,000-row boundary. Open it from Files to inspect bounded windows."
      />
    );
  }
  if (
    props.mode === "worktree" &&
    files.length === 0 &&
    (changes.data?.data.length ?? 0) > 0
  ) {
    return (
      <ReviewState
        title="Diff material is unavailable"
        detail="Git reported changed file identities but produced no reviewable patch."
      />
    );
  }
  if (files.length === 0) {
    return (
      <ReviewState
        title={props.mode === "base" ? "No branch changes" : "Workspace is clean"}
        detail={
          props.mode === "base"
            ? "The workspace matches the merge-base of the default branch."
            : "Tracked and untracked files match the current HEAD."
        }
      />
    );
  }

  return (
    <section className="workspace-review-shell" aria-label="Workspace review">
      <ReviewToolbar
        files={files}
        changes={props.mode === "worktree" ? changes.data?.data : undefined}
        mode={props.mode}
        layout={props.layout}
        navigatorOpen={props.navigatorOpen}
        onModeChange={props.onModeChange}
        onLayoutChange={props.onLayoutChange}
        onNavigatorOpenChange={props.onNavigatorOpenChange}
      />
      {diff.data.truncated ? (
        <p className="review-warning" role="status">
          Review stopped at the 100,000-row boundary; every visible file is
          complete.
        </p>
      ) : null}
      <div className="workspace-review" data-navigator-open={props.navigatorOpen}>
        {props.navigatorOpen ? (
          <FileNavigator
            files={files}
            selectedPath={selectedPath}
            onSelect={props.onSelect}
          />
        ) : null}
        <div ref={scrollRef} className="workspace-diff-scroll">
          {files.map((file) => (
            <FileReview
              key={file.path}
              file={file}
              layout={props.layout}
              selected={file.path === selectedPath}
              collapsed={props.collapsedPaths.includes(file.path)}
              onSelect={props.onSelect}
              onToggleCollapsed={props.onToggleCollapsed}
              onOpenFile={props.onOpenFile}
            />
          ))}
        </div>
      </div>
    </section>
  );
}

function ReviewToolbar(props: {
  files: FileDiff[];
  changes: WorkspaceFileChange[] | undefined;
  mode: ReviewMode;
  layout: DiffLayout;
  navigatorOpen: boolean;
  onModeChange(mode: ReviewMode): void;
  onLayoutChange(layout: DiffLayout): void;
  onNavigatorOpenChange(open: boolean): void;
}) {
  return (
    <header className="review-toolbar">
      <small>{summarizeFiles(props.files, props.changes)}</small>
      <div>
        <ButtonGroup label="Review baseline">
          <ToggleButton
            active={props.mode === "worktree"}
            label="Worktree"
            onClick={() => props.onModeChange("worktree")}
          />
          <ToggleButton
            active={props.mode === "base"}
            label="Branch"
            onClick={() => props.onModeChange("base")}
          />
        </ButtonGroup>
        <ButtonGroup label="Diff layout">
          <ToggleButton
            active={props.layout === "unified"}
            label="Unified"
            onClick={() => props.onLayoutChange("unified")}
          />
          <ToggleButton
            active={props.layout === "split"}
            label="Split"
            onClick={() => props.onLayoutChange("split")}
          />
        </ButtonGroup>
        <button
          type="button"
          aria-pressed={props.navigatorOpen}
          onClick={() => props.onNavigatorOpenChange(!props.navigatorOpen)}
        >
          Files
        </button>
      </div>
    </header>
  );
}

function ButtonGroup(props: { label: string; children: ReactNode }) {
  return (
    <span className="review-button-group" role="group" aria-label={props.label}>
      {props.children}
    </span>
  );
}

function ToggleButton(props: {
  active: boolean;
  label: string;
  onClick(): void;
}) {
  return (
    <button type="button" aria-pressed={props.active} onClick={props.onClick}>
      {props.label}
    </button>
  );
}

function FileNavigator(props: {
  files: FileDiff[];
  selectedPath: string | undefined;
  onSelect(path: string): void;
}) {
  return (
    <aside className="workspace-change-list" aria-label="Changed files">
      {props.files.map((file) => (
        <button
          key={file.path}
          className="workspace-change-row"
          type="button"
          data-selected={file.path === props.selectedPath}
          aria-pressed={file.path === props.selectedPath}
          title={file.path}
          onClick={() => props.onSelect(file.path)}
        >
          <StatusMark status={file.status} />
          <span>
            <strong>{baseName(file.path)}</strong>
            <small>{renameLabel(file)}</small>
          </span>
          <ChangeCount change={file} />
        </button>
      ))}
    </aside>
  );
}

function FileReview(props: {
  file: FileDiff;
  layout: DiffLayout;
  selected: boolean;
  collapsed: boolean;
  onSelect(path: string): void;
  onToggleCollapsed(path: string): void;
  onOpenFile(path: string): void;
}) {
  const toggle = () => {
    props.onSelect(props.file.path);
    props.onToggleCollapsed(props.file.path);
  };
  return (
    <article
      className="file-review"
      data-diff-file={props.file.path}
      data-selected={props.selected}
      aria-label={`Diff for ${props.file.path}`}
    >
      <header>
        <button
          className="file-review-toggle"
          type="button"
          aria-expanded={!props.collapsed}
          onClick={toggle}
        >
          <StatusMark status={props.file.status} />
          <span>
            <strong>{baseName(props.file.path)}</strong>
            <small title={props.file.path}>{renameLabel(props.file)}</small>
          </span>
          <ChangeCount change={props.file} />
          <b aria-hidden="true">{props.collapsed ? "›" : "⌄"}</b>
        </button>
        {props.file.status !== "deleted" ? (
          <button type="button" onClick={() => props.onOpenFile(props.file.path)}>
            Open file
          </button>
        ) : null}
      </header>
      {!props.collapsed ? (
        <FileDiffMaterial file={props.file} layout={props.layout} />
      ) : null}
    </article>
  );
}

function StatusMark({ status }: { status: string }) {
  return (
    <i className="file-status-mark" data-status={status}>
      {statusMark(status)}
    </i>
  );
}

function ChangeCount({ change }: { change: FileDiff }) {
  if (change.binary) return <small className="change-count">binary</small>;
  if (change.added === undefined || change.removed === undefined) {
    return <small className="change-count">counts unavailable</small>;
  }
  return (
    <small className="change-count tabular">
      <b>+{change.added}</b>
      <i>−{change.removed}</i>
    </small>
  );
}

function FileDiffMaterial(props: { file: FileDiff; layout: DiffLayout }) {
  if (props.file.binary) {
    return <p className="file-diff-empty">Binary material has no line-oriented diff.</p>;
  }
  if (props.file.rows.length === 0) {
    return <p className="file-diff-empty">Git produced no textual rows for this change.</p>;
  }
  return props.layout === "split" ? (
    <SplitDiff rows={props.file.rows} />
  ) : (
    <UnifiedDiff rows={props.file.rows} />
  );
}

function UnifiedDiff({ rows }: { rows: DiffRow[] }) {
  return (
    <div className="diff-rows" aria-label="Changed lines">
      {rows.map((row, index) => (
        <UnifiedLine key={`${index}:${row.type}`} row={row} />
      ))}
    </div>
  );
}

function UnifiedLine({ row }: { row: DiffRow }) {
  if (row.type === "hunk") {
    return <div className="diff-hunk">{row.text}</div>;
  }
  return (
    <div className="diff-line" data-type={row.type}>
      <i>{row.leftLine || ""}</i>
      <i>{row.rightLine || ""}</i>
      <b aria-hidden="true">
        {row.type === "added" ? "+" : row.type === "deleted" ? "−" : " "}
      </b>
      <code>{row.code ?? " "}</code>
    </div>
  );
}

type SplitMaterial =
  | { type: "hunk"; row: DiffRow }
  | {
      type: "code";
      left: DiffRow | undefined;
      right: DiffRow | undefined;
    };

function SplitDiff({ rows }: { rows: DiffRow[] }) {
  return (
    <div className="split-diff" aria-label="Changed lines in split layout">
      {splitMaterial(rows).map((material, index) =>
        material.type === "hunk" ? (
          <div key={index} className="diff-hunk">
            {material.row.text}
          </div>
        ) : (
          <div key={index} className="split-diff-row">
            <SplitCell row={material.left} side="left" />
            <SplitCell row={material.right} side="right" />
          </div>
        ),
      )}
    </div>
  );
}

function SplitCell(props: {
  row: DiffRow | undefined;
  side: "left" | "right";
}) {
  const line = props.side === "left" ? props.row?.leftLine : props.row?.rightLine;
  return (
    <span className="split-diff-cell" data-type={props.row?.type ?? "empty"}>
      <i>{line || ""}</i>
      <b aria-hidden="true">
        {props.row?.type === "added"
          ? "+"
          : props.row?.type === "deleted"
            ? "−"
            : " "}
      </b>
      <code>{props.row?.code ?? " "}</code>
    </span>
  );
}

function splitMaterial(rows: DiffRow[]): SplitMaterial[] {
  const material: SplitMaterial[] = [];
  let deleted: DiffRow[] = [];
  let added: DiffRow[] = [];
  const flushChanges = () => {
    const length = Math.max(deleted.length, added.length);
    for (let index = 0; index < length; index += 1) {
      material.push({ type: "code", left: deleted[index], right: added[index] });
    }
    deleted = [];
    added = [];
  };
  for (const row of rows) {
    if (row.type === "deleted") {
      deleted.push(row);
      continue;
    }
    if (row.type === "added") {
      added.push(row);
      continue;
    }
    flushChanges();
    if (row.type === "hunk") {
      material.push({ type: "hunk", row });
    } else {
      material.push({ type: "code", left: row, right: row });
    }
  }
  flushChanges();
  return material;
}

function ReviewState(props: {
  title: string;
  detail?: string;
  onRetry?: () => void;
}) {
  return (
    <section className="review-state">
      <strong>{props.title}</strong>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.onRetry ? (
        <button type="button" onClick={props.onRetry}>
          Try again
        </button>
      ) : null}
    </section>
  );
}

function summarizeFiles(
  files: FileDiff[],
  changes: WorkspaceFileChange[] | undefined,
) {
  const source = changes ?? files;
  const additions = source.reduce(
    (total, change) => total + (change.added ?? 0),
    0,
  );
  const removals = source.reduce(
    (total, change) => total + (change.removed ?? 0),
    0,
  );
  const partial = source.some(
    (change) =>
      change.binary || change.added === undefined || change.removed === undefined,
  );
  return `${source.length} files · +${additions} −${removals}${partial ? " · partial" : ""}`;
}

function statusMark(status: string) {
  switch (status) {
    case "added":
      return "A";
    case "deleted":
      return "D";
    case "renamed":
      return "R";
    case "untracked":
      return "?";
    default:
      return "M";
  }
}

function renameLabel(change: { path: string; previousPath?: string }) {
  return change.previousPath
    ? `${change.previousPath} → ${change.path}`
    : change.path;
}

function baseName(path: string) {
  return path.split("/").at(-1) || path;
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "The review request failed.";
}
