import { describe, expect, it } from "vitest";
import {
  scopeLabelKey,
  workspaceAgentDocsViewModel,
  workspaceKnowledgeViewModel,
  workspaceRecipesViewModel,
  workspaceSkillsViewModel,
} from "./workspaceCatalogViewModel";

describe("workspace catalog view models", () => {
  it("gates knowledge rows when the runtime capability is off", () => {
    expect(
      workspaceKnowledgeViewModel(
        [
          {
            scope: "cwd",
            content: "knowledge",
            revision: "rev-1",
            updatedAt: "2026-01-01T00:00:00Z",
          },
        ],
        false,
      ),
    ).toEqual({
      rows: [],
      count: 0,
      enabled: false,
      isEmpty: true,
    });
  });

  it("projects knowledge row identity and scope labels", () => {
    expect(
      workspaceKnowledgeViewModel(
        [{ scope: "projectRoot", content: "knowledge", revision: "rev-1" }],
        true,
      ),
    ).toEqual({
      rows: [
        {
          id: "projectRoot",
          scope: "projectRoot",
          scopeLabelKey: "knowledge.scope.projectRoot",
          path: "project/LYRA.md",
          content: "knowledge",
          revision: "rev-1",
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
        [{ name: "review", description: "Review code", scope: "project" as const }],
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
        [{ name: "review", description: "Review code", scope: "project" as const }],
        true,
      ).rows,
    ).toEqual([{ id: "review", name: "review", description: "Review code", scope: "project" }]);

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
      {
        id: "AGENTS.md",
        title: "AGENTS.md",
        path: "AGENTS.md",
        scopeLabelKey: "knowledge.scope.cwd",
      },
      {
        id: "root/AGENTS.md",
        title: "Root rules",
        path: "root/AGENTS.md",
        scopeLabelKey: "knowledge.scope.projectRoot",
      },
    ]);
  });

  it("falls back to raw unknown scope labels", () => {
    expect(scopeLabelKey("workspace")).toBe("workspace");
  });
});
