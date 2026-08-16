import { describe, expect, it } from "vitest";
import type { WorkspaceDiff } from "./workspaceQueries";
import {
  workspaceDiffFileHeader,
  workspaceDiffViewModel,
  type WorkspaceFileDiff,
} from "./diffViewModel";

const file = (over: Partial<WorkspaceFileDiff>): WorkspaceFileDiff => ({
  path: "src/app.ts",
  status: "modified",
  added: 0,
  removed: 0,
  rows: [],
  ...over,
});

describe("workspaceDiffViewModel", () => {
  it("projects a missing diff before data resolves", () => {
    expect(workspaceDiffViewModel(undefined)).toEqual({
      files: undefined,
      truncated: false,
    });
  });

  it("totals diff stats across files, counting a missing count as zero", () => {
    const data: WorkspaceDiff = {
      files: [
        file({ path: "src/a.ts", added: 3, removed: 1 }),
        file({ path: "src/b.ts", added: undefined, removed: 4 }),
      ],
      truncated: true,
    };

    expect(workspaceDiffViewModel(data)).toEqual({
      files: data.files,
      subtext: {
        added: 3,
        removed: 5,
        fileCount: 2,
      },
      truncated: true,
    });
  });

  it("reports a complete diff as untruncated", () => {
    expect(workspaceDiffViewModel({ files: [file({ path: "src/only.ts" })] })).toMatchObject({
      truncated: false,
    });
  });
});

describe("workspaceDiffFileHeader", () => {
  it("projects plain and renamed file header labels", () => {
    expect(workspaceDiffFileHeader(file({ path: "src/app.ts", added: 2, removed: 1 }))).toEqual({
      path: "src/app.ts",
      added: 2,
      removed: 1,
    });

    // The rename keeps BOTH paths — the header decides how to spend the width
    // between them, which it cannot do from a pre-joined string.
    expect(
      workspaceDiffFileHeader(
        file({ path: "src/new.ts", previousPath: "src/old.ts", added: undefined, removed: 3 }),
      ),
    ).toEqual({
      path: "src/new.ts",
      previousPath: "src/old.ts",
      added: undefined,
      removed: 3,
    });
  });

  it("omits previousPath entirely when the file was not renamed", () => {
    expect(workspaceDiffFileHeader(file({ path: "src/app.ts" }))).not.toHaveProperty(
      "previousPath",
    );
  });
});
