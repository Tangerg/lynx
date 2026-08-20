import { describe, expect, it } from "vitest";
import type { ToolPreviewComponent } from "@/plugins/sdk";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import {
  askUserToolPreview,
  applyPatchToolPreview,
  fileToolPreview,
  globToolPreview,
  goalToolPreviews,
  grepToolPreview,
  httpToolPreviews,
  lspToolPreview,
  planToolPreviews,
  recallToolPreviews,
  scheduleToolPreviews,
  shellToolPreviews,
  skillToolPreviews,
  delegationToolPreview,
  toolSearchPreview,
  webSearchToolPreview,
  type ToolPreviewContribution,
} from "./toolPreviewContributions";

function independent<const Keys extends readonly string[]>(
  keys: Keys,
): Record<Keys[number], ToolPreviewComponent> {
  return Object.fromEntries(
    keys.map((key) => [
      key,
      function IndependentToolPreview() {
        return null;
      },
    ]),
  ) as unknown as Record<Keys[number], ToolPreviewComponent>;
}

function one(key: string): ToolPreviewComponent {
  return independent([key] as const)[key]!;
}

const keys = (items: ToolPreviewContribution[]) => items.map((item) => item.key);

function allKnownPreviews(): ToolPreviewContribution[] {
  return [
    ...askUserToolPreview(one("ask_user")),
    ...applyPatchToolPreview(independent(["apply_patch"] as const)),
    ...fileToolPreview(one("read")),
    ...globToolPreview(one("glob")),
    ...grepToolPreview(one("grep")),
    ...httpToolPreviews(independent(["http_request", "web_fetch"] as const)),
    ...lspToolPreview(one("lsp")),
    ...planToolPreviews(independent(["enter_plan_mode", "set_plan", "exit_plan_mode"] as const)),
    ...goalToolPreviews(independent(["create_goal", "get_goal", "report_goal_outcome"] as const)),
    ...recallToolPreviews(
      independent(["search_memory", "search_conversations", "read_tool_result"] as const),
    ),
    ...scheduleToolPreviews(
      independent(["create_schedule", "list_schedules", "delete_schedule"] as const),
    ),
    ...shellToolPreviews(independent(["shell", "read_shell_output", "stop_shell"] as const)),
    ...skillToolPreviews(
      independent(["list_skills", "load_skill", "read_skill_resource", "propose_skill"] as const),
    ),
    ...delegationToolPreview(one("delegate_task")),
    ...toolSearchPreview(one("search_tools")),
    ...webSearchToolPreview(one("web_search")),
  ];
}

describe("tool preview contributions", () => {
  it("maps every family to the runtime tool identities it owns", () => {
    expect(
      keys(shellToolPreviews(independent(["shell", "read_shell_output", "stop_shell"] as const))),
    ).toEqual(["shell", "read_shell_output", "stop_shell"]);
    expect(keys(applyPatchToolPreview(independent(["apply_patch"] as const)))).toEqual([
      "apply_patch",
    ]);
    expect(
      keys(
        skillToolPreviews(
          independent([
            "list_skills",
            "load_skill",
            "read_skill_resource",
            "propose_skill",
          ] as const),
        ),
      ),
    ).toEqual(["list_skills", "load_skill", "read_skill_resource", "propose_skill"]);
    expect(
      keys(
        planToolPreviews(independent(["enter_plan_mode", "set_plan", "exit_plan_mode"] as const)),
      ),
    ).toEqual(["enter_plan_mode", "set_plan", "exit_plan_mode"]);
    expect(
      keys(
        goalToolPreviews(independent(["create_goal", "get_goal", "report_goal_outcome"] as const)),
      ),
    ).toEqual(["create_goal", "get_goal", "report_goal_outcome"]);
  });

  it("gives every known tool exactly one specialised renderer", () => {
    const previews = allKnownPreviews();
    const names = keys(previews);

    expect(names.slice().sort()).toEqual(Object.keys(TOOL_ICON_BY_NAME).sort());
    expect(new Set(names).size).toBe(names.length);
  });

  it("keeps every known tool on an independent component boundary", () => {
    const previews = allKnownPreviews();

    expect(new Set(previews.map((preview) => preview.component)).size).toBe(previews.length);
  });
});
