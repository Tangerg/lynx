import { describe, expect, it } from "vitest";
import {
  codebaseSearchViewModel,
  codebaseStatusViewModel,
  scopeLabel,
  workspaceAgentDocsViewModel,
  workspaceMemoryViewModel,
  workspaceRecipesViewModel,
  workspaceSkillsViewModel,
} from "./workspaceCatalogViewModel";

describe("workspace catalog view models", () => {
  it("gates memory rows when the runtime capability is off", () => {
    expect(
      workspaceMemoryViewModel(
        [{ scope: "cwd", path: "LYRA.md", content: "memory", updatedAt: "2026-01-01T00:00:00Z" }],
        false,
      ),
    ).toEqual({
      rows: [],
      count: 0,
      enabled: false,
      isEmpty: true,
    });
  });

  it("projects memory row identity and scope labels", () => {
    expect(
      workspaceMemoryViewModel(
        [{ scope: "projectRoot", path: "project/LYRA.md", content: "memory" }],
        true,
      ),
    ).toEqual({
      rows: [
        {
          id: "projectRoot:project/LYRA.md",
          scope: "projectRoot",
          scopeLabel: "project root",
          path: "project/LYRA.md",
          content: "memory",
          updatedAt: undefined,
        },
      ],
      count: 1,
      enabled: true,
      isEmpty: false,
    });
  });

  it("gates skills rows when the runtime capability is off", () => {
    expect(
      workspaceSkillsViewModel(
        [{ name: "review", description: "Review code", source: "project" }],
        false,
      ),
    ).toMatchObject({
      rows: [],
      enabled: false,
      isEmpty: true,
    });
  });

  it("projects skills, recipes, and agent docs into stable rows", () => {
    expect(
      workspaceSkillsViewModel(
        [{ name: "review", description: "Review code", source: "project" }],
        true,
      ).rows,
    ).toEqual([{ id: "review", name: "review", description: "Review code", source: "project" }]);

    expect(
      workspaceRecipesViewModel([
        {
          name: "fix",
          argumentHint: "<file>",
          description: "Fix a file",
          source: "project",
          scope: "project",
        },
      ]).rows,
    ).toEqual([
      {
        id: "project:fix",
        command: "/fix",
        argumentHint: "<file>",
        description: "Fix a file",
        scope: "project",
      },
    ]);

    expect(
      workspaceAgentDocsViewModel([
        { path: "AGENTS.md", title: "", scope: "cwd" },
        { path: "root/AGENTS.md", title: "Root rules", scope: "projectRoot" },
      ]).rows,
    ).toEqual([
      { id: "AGENTS.md", title: "AGENTS.md", path: "AGENTS.md", scopeLabel: "cwd" },
      {
        id: "root/AGENTS.md",
        title: "Root rules",
        path: "root/AGENTS.md",
        scopeLabel: "project root",
      },
    ]);
  });

  it("falls back to raw unknown scope labels", () => {
    expect(scopeLabel("workspace")).toBe("workspace");
  });
});

describe("codebase view models", () => {
  it("normalizes missing and unknown status to the none state", () => {
    expect(codebaseStatusViewModel(undefined)).toEqual({
      state: "none",
      fileCount: 0,
      chunkCount: 0,
    });
    expect(codebaseStatusViewModel({ state: "paused", fileCount: 4, chunkCount: 8 })).toEqual({
      state: "none",
      fileCount: 4,
      chunkCount: 8,
    });
  });

  it("projects codebase hit display fields", () => {
    expect(
      codebaseSearchViewModel([
        {
          path: "src/app.ts",
          startLine: 3,
          endLine: 7,
          snippet: "const app = true",
          score: 0.876,
        },
      ]),
    ).toEqual({
      rows: [
        {
          id: "src/app.ts:3:7:0",
          pathRange: "src/app.ts:3-7",
          score: "0.88",
          snippet: "const app = true",
        },
      ],
      isEmpty: false,
    });
  });

  it("distinguishes no search from an empty result", () => {
    expect(codebaseSearchViewModel(null)).toEqual({ rows: [], isEmpty: false });
    expect(codebaseSearchViewModel([])).toEqual({ rows: [], isEmpty: true });
  });
});
