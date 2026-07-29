// The changed-file navigator's read model.
//
// Reviewing a multi-file change needs two things at once: every file's diff in
// one scroll, and a way to reach a file without scrolling past the others. The
// diff list already answers the first; this folds the same flat file list into
// the tree the navigator renders. It is the diff's own file set — NOT the
// working tree — so nothing is fetched and nothing lazy-loads: the whole shape
// is known the moment the diff arrives.

import type { WorkspaceFileDiff } from "./workspaceData";

export interface ReviewTreeFileNode {
  kind: "file";
  /** Final path segment — what the row shows. */
  name: string;
  /** Full repo-relative path: the selection id, the React key, and the scroll
   *  anchor the diff list marks each file card with. */
  path: string;
}

export interface ReviewTreeDirectoryNode {
  kind: "directory";
  /** Display name; a compressed chain reads as `parent/child`. */
  name: string;
  path: string;
  children: ReviewTreeNode[];
}

export type ReviewTreeNode = ReviewTreeDirectoryNode | ReviewTreeFileNode;

interface MutableDirectory {
  name: string;
  path: string;
  directories: Map<string, MutableDirectory>;
  files: ReviewTreeFileNode[];
}

function directory(name: string, path: string): MutableDirectory {
  return { name, path, directories: new Map(), files: [] };
}

// Natural order, case-insensitive: `item2` before `item10`, and a capitalised
// name does not sort into its own block ahead of the lowercase ones.
function compareName(left: string, right: string): number {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" });
}

// Collapse an unbranched directory chain into one row (`src` → `plugins` reads
// as `src/plugins`). A repo-relative diff path is mostly unbranched prefix, so
// without this the navigator spends its first four rows saying nothing.
function compress(node: ReviewTreeDirectoryNode): ReviewTreeDirectoryNode {
  let current = node;
  while (current.children.length === 1) {
    const onlyChild = current.children[0];
    if (!onlyChild || onlyChild.kind !== "directory") break;
    current = {
      kind: "directory",
      name: `${current.name}/${onlyChild.name}`,
      path: onlyChild.path,
      children: onlyChild.children,
    };
  }
  return current;
}

function finalize(node: MutableDirectory): ReviewTreeNode[] {
  const directories: ReviewTreeDirectoryNode[] = [];
  for (const child of node.directories.values()) {
    directories.push(
      compress({
        kind: "directory",
        name: child.name,
        path: child.path,
        children: finalize(child),
      }),
    );
  }
  // Directories first, then files — the ordering every file explorer uses, so
  // the navigator and the workspace tree read the same way. Both arrays were
  // built right here, so sorting them in place aliases nothing.
  directories.sort((left, right) => compareName(left.name, right.name));
  node.files.sort((left, right) => compareName(left.name, right.name));
  return [...directories, ...node.files];
}

/** Fold a diff's flat file list into a sorted, path-compressed tree. */
export function buildReviewFileTree(files: readonly WorkspaceFileDiff[]): ReviewTreeNode[] {
  const root = directory("", "");
  for (const file of files) {
    const segments = file.path.split("/").filter((segment) => segment.length > 0);
    const name = segments.at(-1);
    if (name === undefined) continue;
    let parent = root;
    for (const segment of segments.slice(0, -1)) {
      const path = parent.path ? `${parent.path}/${segment}` : segment;
      let child = parent.directories.get(segment);
      if (!child) {
        child = directory(segment, path);
        parent.directories.set(segment, child);
      }
      parent = child;
    }
    parent.files.push({ kind: "file", name, path: file.path });
  }
  return finalize(root);
}

/** Substring match on the whole path, so `views/diff` narrows by directory as
 *  well as by filename. An empty query keeps every file. */
export function filterReviewFiles(
  files: readonly WorkspaceFileDiff[],
  query: string,
): WorkspaceFileDiff[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return [...files];
  return files.filter((file) => file.path.toLowerCase().includes(needle));
}
