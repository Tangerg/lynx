import type { WorkspaceFileChange } from "./workspaceData";

export interface FileChangeTag {
  letter: "A" | "D" | "M";
  className: string;
}

export type FileChangeLineStats =
  { kind: "binary" } | { kind: "text"; added: number; removed: number };

export interface FileChangeRowViewModel {
  path: string;
  active: boolean;
  tag: FileChangeTag;
  lineStats: FileChangeLineStats;
}

export interface FileChangesViewModel {
  rows: FileChangeRowViewModel[];
  fileCount: number;
  totalAdded: number;
  totalRemoved: number;
  isEmpty: boolean;
}

const TAG_BY_CHANGE: Record<WorkspaceFileChange["change"], FileChangeTag> = {
  add: { className: "text-success", letter: "A" },
  del: { className: "text-negative", letter: "D" },
  mod: { className: "text-warning", letter: "M" },
};

export function fileChangesViewModel(
  files: readonly WorkspaceFileChange[],
  activePath = "",
): FileChangesViewModel {
  let totalAdded = 0;
  let totalRemoved = 0;

  const rows = files.map((file): FileChangeRowViewModel => {
    const added = file.added ?? 0;
    const removed = file.removed ?? 0;
    totalAdded += added;
    totalRemoved += removed;

    return {
      path: file.path,
      active: file.path === activePath,
      tag: TAG_BY_CHANGE[file.change],
      lineStats: file.binary ? { kind: "binary" } : { kind: "text", added, removed },
    };
  });

  return {
    rows,
    fileCount: files.length,
    totalAdded,
    totalRemoved,
    isEmpty: files.length === 0,
  };
}

export function fileChangesSubtext({ fileCount }: Pick<FileChangesViewModel, "fileCount">): string {
  return `${fileCount} files · uncommitted`;
}
