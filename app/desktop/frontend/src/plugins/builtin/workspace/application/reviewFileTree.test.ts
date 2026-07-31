import { describe, expect, it } from "vitest";
import type { WorkspaceFileDiff } from "./workspaceQueries";
import { buildReviewFileTree, filterReviewFiles } from "./reviewFileTree";

const file = (path: string): WorkspaceFileDiff => ({ path, status: "modified", rows: [] });

describe("buildReviewFileTree", () => {
  it("has no rows for a diff with no files", () => {
    expect(buildReviewFileTree([])).toEqual([]);
  });

  it("keeps a root-level file at the root", () => {
    expect(buildReviewFileTree([file("README.md")])).toEqual([
      { kind: "file", name: "README.md", path: "README.md" },
    ]);
  });

  it("compresses an unbranched directory chain into one row", () => {
    expect(buildReviewFileTree([file("src/plugins/workspace/diff.tsx")])).toEqual([
      {
        kind: "directory",
        name: "src/plugins/workspace",
        path: "src/plugins/workspace",
        children: [{ kind: "file", name: "diff.tsx", path: "src/plugins/workspace/diff.tsx" }],
      },
    ]);
  });

  it("stops compressing where the chain branches, and compresses again below it", () => {
    expect(buildReviewFileTree([file("src/ui/atoms/chip.tsx"), file("src/lib/path.ts")])).toEqual([
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

    expect(tree).toEqual([
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
