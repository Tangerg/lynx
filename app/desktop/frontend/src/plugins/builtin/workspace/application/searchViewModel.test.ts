import { describe, expect, it } from "vitest";
import type { WorkspaceGrepMatch } from "./workspaceData";
import { workspaceSearchSubtext, workspaceSearchViewModel } from "./searchViewModel";

const match = (over: Partial<WorkspaceGrepMatch>): WorkspaceGrepMatch => ({
  path: "src/app.ts",
  lineNumber: 1,
  text: "needle",
  ...over,
});

describe("workspaceSearchViewModel", () => {
  it("projects a missing result before the query resolves", () => {
    expect(workspaceSearchViewModel(undefined)).toEqual({
      groups: [],
      totalCount: 0,
      shownCount: 0,
      overflowCount: 0,
      hasResult: false,
    });
  });

  it("groups matches by file while preserving first-seen file order", () => {
    const first = match({ path: "src/a.ts", lineNumber: 1 });
    const second = match({ path: "src/b.ts", lineNumber: 2 });
    const third = match({ path: "src/a.ts", lineNumber: 3 });

    expect(workspaceSearchViewModel({ matches: [first, second, third], total: 5 })).toEqual({
      groups: [
        { path: "src/a.ts", matches: [first, third], matchCount: 2 },
        { path: "src/b.ts", matches: [second], matchCount: 1 },
      ],
      totalCount: 5,
      shownCount: 3,
      overflowCount: 2,
      hasResult: true,
    });
  });

  it("never reports a negative overflow when total is smaller than shown matches", () => {
    expect(
      workspaceSearchViewModel({
        matches: [match({ lineNumber: 1 }), match({ lineNumber: 2 })],
        total: 1,
      }).overflowCount,
    ).toBe(0);
  });
});

describe("workspaceSearchSubtext", () => {
  it("omits subtext before a result exists", () => {
    expect(workspaceSearchSubtext(workspaceSearchViewModel(null))).toBeUndefined();
  });

  it("builds match count subtext", () => {
    expect(workspaceSearchSubtext(workspaceSearchViewModel({ matches: [], total: 4 }))).toBe(
      "4 matches",
    );
  });
});
