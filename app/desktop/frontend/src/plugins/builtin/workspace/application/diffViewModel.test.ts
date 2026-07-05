import { describe, expect, it } from "vitest";
import type { WorkspaceDiff } from "@/lib/data/queries";
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
      shouldShowFileHeaders: false,
    });
  });

  it("totals diff stats and marks multi-file headers", () => {
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
      shouldShowFileHeaders: true,
    });
  });

  it("does not show per-file headers for a single-file diff", () => {
    expect(workspaceDiffViewModel({ files: [file({ path: "src/only.ts" })] })).toMatchObject({
      shouldShowFileHeaders: false,
    });
  });
});

describe("workspaceDiffFileHeader", () => {
  it("projects plain and renamed file header labels", () => {
    expect(workspaceDiffFileHeader(file({ path: "src/app.ts", added: 2, removed: 1 }))).toEqual({
      displayPath: "src/app.ts",
      added: 2,
      removed: 1,
    });

    expect(
      workspaceDiffFileHeader(
        file({ path: "src/new.ts", previousPath: "src/old.ts", added: undefined, removed: 3 }),
      ),
    ).toEqual({
      displayPath: "src/old.ts → src/new.ts",
      added: undefined,
      removed: 3,
    });
  });
});
