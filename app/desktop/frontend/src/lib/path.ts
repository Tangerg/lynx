// Path DISPLAY helpers — how a cwd reads in chrome (chips, tree nodes),
// not filesystem logic. The runtime owns real path semantics (jail,
// normalization); the UI only ever shortens what the server returned.

/** Last segment of a directory path ("/a/b/c/" → "c"); the input itself
 *  when there's nothing to split (root, ""). */
export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) || path;
}

/**
 * A path split for a two-line row: the name you scan for, and the directory you
 * check once you have found it.
 *
 * `directory` is "" for a path with no separator, which is the signal for "no second
 * line" — an empty one would reserve the height and say nothing.
 */
export function splitFilePath(path: string): { directory: string; name: string } {
  const cut = path.replace(/\/+$/, "").lastIndexOf("/");
  if (cut < 0) return { directory: "", name: path };
  return { directory: path.slice(0, cut), name: path.slice(cut + 1) };
}
