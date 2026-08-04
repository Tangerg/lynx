import { describe, expect, it } from "vitest";
import type { WorkspaceFileDiff } from "./workspaceQueries";
import { buildReviewFileTree, filterReviewFiles, type ReviewTreeNode } from "./reviewFileTree";

const file = (path: string, stat: Partial<WorkspaceFileDiff> = {}): WorkspaceFileDiff => ({
  path,
  status: "modified",
  rows: [],
  ...stat,
});

/** Structure only. Every node also carries its line counts, which have their own
 *  tests below — asserting them here would bury what these cases are about. */
const shape = (nodes: readonly ReviewTreeNode[]): unknown =>
  nodes.map((node) =>
    node.kind === "directory"
      ? { kind: node.kind, name: node.name, path: node.path, children: shape(node.children) }
      : { kind: node.kind, name: node.name, path: node.path },
  );

describe("buildReviewFileTree", () => {
  it("has no rows for a diff with no files", () => {
    expect(shape(buildReviewFileTree([]))).toEqual([]);
  });

  it("keeps a root-level file at the root", () => {
    expect(shape(buildReviewFileTree([file("README.md")]))).toEqual([
      { kind: "file", name: "README.md", path: "README.md" },
    ]);
  });

  it("compresses an unbranched directory chain into one row", () => {
    expect(shape(buildReviewFileTree([file("src/plugins/workspace/diff.tsx")]))).toEqual([
      {
        kind: "directory",
        name: "src/plugins/workspace",
        path: "src/plugins/workspace",
        children: [{ kind: "file", name: "diff.tsx", path: "src/plugins/workspace/diff.tsx" }],
      },
    ]);
  });

  it("stops compressing where the chain branches, and compresses again below it", () => {
    expect(
      shape(buildReviewFileTree([file("src/ui/atoms/chip.tsx"), file("src/lib/path.ts")])),
    ).toEqual([
      {
        kind: "directory",
        name: "src",
        path: "src",
        children: [
          {
            kind: "directory",
            name: "lib",
            path: "src/lib",
            children: [{ kind: "file", name: "path.ts", path: "src/lib/path.ts" }],
          },
          {
            kind: "directory",
            name: "ui/atoms",
            path: "src/ui/atoms",
            children: [{ kind: "file", name: "chip.tsx", path: "src/ui/atoms/chip.tsx" }],
          },
        ],
      },
    ]);
  });

  it("orders directories before files, each in natural case-insensitive order", () => {
    const tree = buildReviewFileTree([
      file("zebra.ts"),
      file("Apple.ts"),
      file("item10.ts"),
      file("item2.ts"),
      file("ui/one.ts"),
      file("lib/two.ts"),
    ]);

    expect(tree.map((node) => node.name)).toEqual([
      "lib",
      "ui",
      "Apple.ts",
      "item2.ts",
      "item10.ts",
      "zebra.ts",
    ]);
  });

  it("keeps a file and a same-named directory as separate rows", () => {
    // A change that replaces a file with a directory (delete `notes`, add
    // `notes/index.md`) yields both, and neither may swallow the other.
    const tree = buildReviewFileTree([file("notes"), file("notes/index.md")]);

    expect(shape(tree)).toEqual([
      {
        kind: "directory",
        name: "notes",
        path: "notes",
        children: [{ kind: "file", name: "index.md", path: "notes/index.md" }],
      },
      { kind: "file", name: "notes", path: "notes" },
    ]);
  });
});

describe("buildReviewFileTree · line counts", () => {
  it("carries each file's own counts", () => {
    const [node] = buildReviewFileTree([file("a.ts", { added: 12, removed: 3 })]);
    expect(node).toMatchObject({ kind: "file", added: 12, removed: 3, binary: false });
  });

  it("treats a missing count as zero rather than as unknown", () => {
    const [node] = buildReviewFileTree([file("a.ts")]);
    expect(node).toMatchObject({ added: 0, removed: 0 });
  });

  // The figure a collapsed directory shows is the only one it can honestly show.
  it("rolls a directory's total up from everything beneath it", () => {
    const tree = buildReviewFileTree([
      file("src/ui/a.ts", { added: 10, removed: 1 }),
      file("src/lib/b.ts", { added: 5, removed: 20 }),
    ]);

    expect(tree[0]).toMatchObject({ kind: "directory", name: "src", added: 15, removed: 21 });
  });

  it("keeps a binary file's absence of counts, and does not let it hide a sibling's", () => {
    const [node] = buildReviewFileTree([file("logo.png", { binary: true })]);
    expect(node).toMatchObject({ binary: true, added: 0, removed: 0 });

    // A directory holding one binary file and one edited file HAS counts, so it is
    // not binary — the flag only survives where there is nothing else to report.
    const tree = buildReviewFileTree([
      file("assets/logo.png", { binary: true }),
      file("assets/index.ts", { added: 4, removed: 0 }),
    ]);
    expect(tree[0]).toMatchObject({ kind: "directory", added: 4, removed: 0, binary: false });
  });
});

describe("filterReviewFiles", () => {
  const files = [file("src/ui/chip.tsx"), file("src/lib/path.ts"), file("README.md")];

  it("keeps every file when the query is blank", () => {
    expect(filterReviewFiles(files, "   ")).toHaveLength(3);
  });

  it("matches on the whole path, so a directory narrows too", () => {
    expect(filterReviewFiles(files, "src/lib").map((f) => f.path)).toEqual(["src/lib/path.ts"]);
  });

  it("ignores case", () => {
    expect(filterReviewFiles(files, "readme").map((f) => f.path)).toEqual(["README.md"]);
  });
});
