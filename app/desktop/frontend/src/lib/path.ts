// Path DISPLAY helpers — how a cwd reads in chrome (chips, tree nodes),
// not filesystem logic. The runtime owns real path semantics (jail,
// normalization); the UI only ever shortens what the server returned.

/** Last segment of a directory path ("/a/b/c/" → "c"); the input itself
 *  when there's nothing to split (root, ""). */
export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) || path;
}
