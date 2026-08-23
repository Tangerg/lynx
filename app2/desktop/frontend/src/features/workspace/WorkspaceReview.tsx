import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, type ReactNode } from "react";

import type {
  DiffRow,
  FileDiff,
  RuntimeConnection,
  WorkspaceFileChange,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import { useLocalization, type Translate } from "../localization/Localization";
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
  const { t } = useLocalization();
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
    return <ReviewState title={t("review.building")} />;
  }
  if (diff.isError || worktreeError) {
    const error = diff.isError ? diff.error : changes.error;
    return (
      <ReviewState
        title={
          props.mode === "base"
            ? t("review.branchUnavailable")
            : t("review.loadFailed")
        }
        detail={messageOf(error, t("review.requestFailed"))}
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
        title={t("review.boundary")}
        detail={t("review.boundaryDetail")}
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
        title={t("review.materialUnavailable")}
        detail={t("review.materialUnavailableDetail")}
      />
    );
  }
  if (files.length === 0) {
    return (
      <ReviewState
        title={
          props.mode === "base"
            ? t("review.noBranchChanges")
            : t("review.clean")
        }
        detail={
          props.mode === "base"
            ? t("review.noBranchChangesDetail")
            : t("review.cleanDetail")
        }
      />
    );
  }

  return (
    <section className="workspace-review-shell" aria-label={t("review.label")}>
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
          {t("review.truncated")}
        </p>
      ) : null}
      <div
        className="workspace-review"
        data-navigator-open={props.navigatorOpen}
      >
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
  const { formatNumber, t } = useLocalization();
  return (
    <header className="review-toolbar">
      <small>
        {summarizeFiles(props.files, props.changes, t, formatNumber)}
      </small>
      <div>
        <ButtonGroup label={t("review.baseline")}>
          <ToggleButton
            active={props.mode === "worktree"}
            label={t("review.worktree")}
            onClick={() => props.onModeChange("worktree")}
          />
          <ToggleButton
            active={props.mode === "base"}
            label={t("review.branch")}
            onClick={() => props.onModeChange("base")}
          />
        </ButtonGroup>
        <ButtonGroup label={t("review.layout")}>
          <ToggleButton
            active={props.layout === "unified"}
            label={t("review.unified")}
            onClick={() => props.onLayoutChange("unified")}
          />
          <ToggleButton
            active={props.layout === "split"}
            label={t("review.split")}
            onClick={() => props.onLayoutChange("split")}
          />
        </ButtonGroup>
        <button
          type="button"
          aria-pressed={props.navigatorOpen}
          onClick={() => props.onNavigatorOpenChange(!props.navigatorOpen)}
        >
          {t("review.files")}
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
  const { t } = useLocalization();
  return (
    <aside
      className="workspace-change-list"
      aria-label={t("review.changedFiles")}
    >
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
  const { t } = useLocalization();
  const toggle = () => {
    props.onSelect(props.file.path);
    props.onToggleCollapsed(props.file.path);
  };
  return (
    <article
      className="file-review"
      data-diff-file={props.file.path}
      data-selected={props.selected}
      aria-label={t("review.diffFor", { path: props.file.path })}
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
            <small dir="ltr" title={props.file.path}>
              {renameLabel(props.file)}
            </small>
          </span>
          <ChangeCount change={props.file} />
          <b aria-hidden="true">{props.collapsed ? "›" : "⌄"}</b>
        </button>
        {props.file.status !== "deleted" ? (
          <button
            type="button"
            onClick={() => props.onOpenFile(props.file.path)}
          >
            {t("review.openFile")}
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
  const { t } = useLocalization();
  if (change.binary) {
    return <small className="change-count">{t("review.binary")}</small>;
  }
  if (change.added === undefined || change.removed === undefined) {
    return (
      <small className="change-count">{t("review.countsUnavailable")}</small>
    );
  }
  return (
    <small className="change-count tabular">
      <b>+{change.added}</b>
      <i>−{change.removed}</i>
    </small>
  );
}

function FileDiffMaterial(props: { file: FileDiff; layout: DiffLayout }) {
  const { t } = useLocalization();
  if (props.file.binary) {
    return <p className="file-diff-empty">{t("review.binaryNoDiff")}</p>;
  }
  if (props.file.rows.length === 0) {
    return <p className="file-diff-empty">{t("review.noTextRows")}</p>;
  }
  return props.layout === "split" ? (
    <SplitDiff rows={props.file.rows} />
  ) : (
    <UnifiedDiff rows={props.file.rows} />
  );
}

function UnifiedDiff({ rows }: { rows: DiffRow[] }) {
  const { t } = useLocalization();
  return (
    <div className="diff-rows" aria-label={t("review.changedLines")}>
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
  const { t } = useLocalization();
  return (
    <div className="split-diff" aria-label={t("review.changedLinesSplit")}>
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
  const line =
    props.side === "left" ? props.row?.leftLine : props.row?.rightLine;
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
      material.push({
        type: "code",
        left: deleted[index],
        right: added[index],
      });
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
  const { t } = useLocalization();
  return (
    <section className="review-state">
      <strong>{props.title}</strong>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.onRetry ? (
        <button type="button" onClick={props.onRetry}>
          {t("review.tryAgain")}
        </button>
      ) : null}
    </section>
  );
}

function summarizeFiles(
  files: FileDiff[],
  changes: WorkspaceFileChange[] | undefined,
  t: Translate,
  formatNumber: (value: number) => string,
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
      change.binary ||
      change.added === undefined ||
      change.removed === undefined,
  );
  return `${t("review.summary", {
    files: formatNumber(source.length),
    additions: formatNumber(additions),
    removals: formatNumber(removals),
  })}${partial ? ` · ${t("review.partial")}` : ""}`;
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

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
