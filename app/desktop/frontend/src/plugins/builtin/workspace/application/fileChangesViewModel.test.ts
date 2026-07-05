import { describe, expect, it } from "vitest";
import type { WorkspaceFileChange } from "./workspaceData";
import { fileChangesSubtext, fileChangesViewModel } from "./fileChangesViewModel";

const change = (over: Partial<WorkspaceFileChange>): WorkspaceFileChange => ({
  path: "src/app.ts",
  change: "mod",
  ...over,
});

describe("fileChangesViewModel", () => {
  it("projects empty file changes", () => {
    expect(fileChangesViewModel([])).toEqual({
      rows: [],
      fileCount: 0,
      totalAdded: 0,
      totalRemoved: 0,
      isEmpty: true,
    });
  });

  it("keeps row order, marks the active file, and totals line changes", () => {
    expect(
      fileChangesViewModel(
        [
          change({ path: "src/new.ts", change: "add", added: 10 }),
          change({ path: "src/old.ts", change: "del", removed: 4 }),
          change({ path: "src/app.ts", change: "mod", added: 3, removed: 2 }),
        ],
        "src/app.ts",
      ),
    ).toEqual({
      rows: [
        {
          path: "src/new.ts",
          active: false,
          tag: { className: "text-success", letter: "A" },
          lineStats: { kind: "text", added: 10, removed: 0 },
        },
        {
          path: "src/old.ts",
          active: false,
          tag: { className: "text-negative", letter: "D" },
          lineStats: { kind: "text", added: 0, removed: 4 },
        },
        {
          path: "src/app.ts",
          active: true,
          tag: { className: "text-warning", letter: "M" },
          lineStats: { kind: "text", added: 3, removed: 2 },
        },
      ],
      fileCount: 3,
      totalAdded: 13,
      totalRemoved: 6,
      isEmpty: false,
    });
  });

  it("projects binary files without fake row line counts", () => {
    expect(
      fileChangesViewModel([change({ path: "asset.png", binary: true, added: 8, removed: 9 })]),
    ).toMatchObject({
      rows: [
        {
          path: "asset.png",
          lineStats: { kind: "binary" },
        },
      ],
      totalAdded: 8,
      totalRemoved: 9,
    });
  });
});

describe("fileChangesSubtext", () => {
  it("builds files header subtext", () => {
    expect(fileChangesSubtext({ fileCount: 2 })).toBe("2 files · uncommitted");
  });
});
