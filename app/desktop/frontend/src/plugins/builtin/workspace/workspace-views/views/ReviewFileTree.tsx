// The review panel's changed-file navigator: a filterable tree of the diff's own
// files, beside the diff rather than instead of it.
//
// Selecting a row does NOT swap the panel's content — it scrolls the diff to that
// file's card. Before this existed, reaching a file meant opening the Files view
// in the dock and clicking a row, which replaced the list with the diff: the one
// thing you need while reviewing (where am I in the change) was the thing the
// click threw away.

import { useState } from "react";
import type { KeyboardEvent, ReactNode } from "react";
import { DiffStat, Icon, IconButton, Pressable, ScrollArea, TextField } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import type { WorkspaceFileDiff } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  type ReviewTreeNode,
  buildReviewFileTree,
  filterReviewFiles,
} from "@/plugins/builtin/workspace/application/reviewFileTree";

/** Row indent per level. The base inset keeps a depth-0 row off the pane edge. */
function indentStyle(depth: number) {
  return { paddingLeft: `${0.5 + depth * 0.75}rem` };
}

/** While filtering, collapse state is ignored — a match inside a closed
 *  directory would otherwise leave the pane looking empty. */
const NOTHING_COLLAPSED: ReadonlySet<string> = new Set();

function TreeRows({
  nodes,
  depth,
  selectedPath,
  collapsedPaths,
  binaryLabel,
  onToggleDirectory,
  onSelectFile,
}: {
  binaryLabel: string;
  nodes: ReviewTreeNode[];
  depth: number;
  selectedPath: string;
  collapsedPaths: ReadonlySet<string>;
  onToggleDirectory: (path: string) => void;
  onSelectFile: (path: string) => void;
}) {
  return nodes.map((node) => {
    if (node.kind === "file") {
      return (
        <TreeRow
          // Namespaced by kind: a change that replaces a file with a directory
          // of the same name yields sibling nodes sharing one path.
          key={`file:${node.path}`}
          depth={depth}
          selected={node.path === selectedPath}
          leading={<Icon name="file" size="sm" className="shrink-0 opacity-70" />}
          label={node.name}
          trailing={
            <DiffStat
              added={node.added}
              removed={node.removed}
              binary={node.binary ? binaryLabel : undefined}
            />
          }
          onClick={() => onSelectFile(node.path)}
        />
      );
    }
    const open = !collapsedPaths.has(node.path);
    return (
      <div key={`dir:${node.path}`}>
        <TreeRow
          depth={depth}
          selected={false}
          leading={
            <Icon
              name="chevron-down"
              size="xs"
              className={cn("shrink-0 transition-transform", !open && "-rotate-90")}
            />
          }
          label={node.name}
          labelClassName="text-fg-muted"
          expanded={open}
          trailing={
            <DiffStat
              added={node.added}
              removed={node.removed}
              binary={node.binary ? binaryLabel : undefined}
            />
          }
          onClick={() => onToggleDirectory(node.path)}
        />
        {open && (
          <TreeRows
            nodes={node.children}
            depth={depth + 1}
            selectedPath={selectedPath}
            collapsedPaths={collapsedPaths}
            binaryLabel={binaryLabel}
            onToggleDirectory={onToggleDirectory}
            onSelectFile={onSelectFile}
          />
        )}
      </div>
    );
  });
}

function TreeRow({
  depth,
  selected,
  leading,
  label,
  labelClassName,
  expanded,
  trailing,
  onClick,
}: {
  depth: number;
  selected: boolean;
  leading: ReactNode;
  label: string;
  labelClassName?: string;
  expanded?: boolean;
  /** The row's figure, right-aligned in a column of its own. */
  trailing?: ReactNode;
  onClick: () => void;
}) {
  return (
    <Pressable
      type="button"
      // The row fills instead of ringing when the keyboard reaches it, which is
      // what the selected row already looks like — arrow-key travel and clicking
      // read as the same gesture.
      data-chrome-focus=""
      aria-expanded={expanded}
      aria-current={selected ? "true" : undefined}
      onClick={onClick}
      style={indentStyle(depth)}
      className={cn(
        "flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md border-0 bg-transparent pr-2",
        "text-left text-ui-xs text-fg transition-colors hover:bg-hover focus-visible:bg-hover",
        selected && "bg-selected",
      )}
      title={label}
    >
      {leading}
      <span className={cn("min-w-0 flex-1 truncate", labelClassName)}>{label}</span>
      {trailing}
    </Pressable>
  );
}

export function ReviewFileTree({
  files,
  selectedPath,
  onSelectFile,
  onClose,
}: {
  files: WorkspaceFileDiff[];
  /** The file the diff is scrolled to, so the tree can say where you are. */
  selectedPath: string;
  onSelectFile: (path: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  const [query, setQuery] = useState("");
  // The diff's file set is known upfront and usually small, so every directory
  // starts open and we track what the user has CLOSED.
  const [collapsedPaths, setCollapsedPaths] = useState<ReadonlySet<string>>(() => new Set());

  const filtered = filterReviewFiles(files, query);
  const nodes = buildReviewFileTree(filtered);
  const filtering = query.trim().length > 0;

  const toggleDirectory = (path: string) => {
    setCollapsedPaths((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  };
  // Escape clears the filter before the view's own Escape handling gets it —
  // otherwise the first press closes the panel and takes the query with it.
  const onFilterKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape" && query.length > 0) {
      event.stopPropagation();
      setQuery("");
    }
  };

  return (
    <aside
      aria-label={t("diff.files.aria")}
      // The navigator's own width, so the panel beside it just takes the rest:
      // wide enough for a nested path at the dock's review width, and a share
      // rather than a fixed number so a narrowed dock leaves the code column
      // something to render into.
      className="agent-pane-split flex h-full min-h-0 w-[min(42%,28rem)] min-w-0 shrink-0 flex-col"
    >
      <div className="agent-surface-divider flex shrink-0 items-center gap-1.5 p-2">
        <TextField
          size="sm"
          font="sans"
          value={query}
          spellCheck={false}
          autoComplete="off"
          placeholder={t("diff.files.filter")}
          aria-label={t("diff.files.filter")}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onFilterKeyDown}
        />
        <IconButton icon="x" size="sm" aria-label={t("diff.files.hide")} onClick={onClose} />
      </div>
      <ScrollArea className="px-1 py-1">
        {nodes.length === 0 ? (
          <p className="m-0 px-2 py-2 text-ui-xs text-fg-faint">
            {filtering ? t("diff.files.noMatch") : t("diff.files.none")}
          </p>
        ) : (
          <TreeRows
            nodes={nodes}
            depth={0}
            selectedPath={selectedPath}
            binaryLabel={t("files.binary")}
            collapsedPaths={filtering ? NOTHING_COLLAPSED : collapsedPaths}
            onToggleDirectory={toggleDirectory}
            onSelectFile={onSelectFile}
          />
        )}
      </ScrollArea>
    </aside>
  );
}
