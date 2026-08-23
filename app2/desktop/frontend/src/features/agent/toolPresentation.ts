import type { Item } from "@lyra/runtime-contract";

import {
  translateEnglish,
  type Translate,
} from "../localization/Localization";

export interface ToolPresentation {
  title: string;
  subject?: string;
  detail?: string;
  kind: "read" | "write" | "exec" | "network" | "control" | "tool";
  glyph: string;
}

export function presentTool(
  name: string,
  argumentsValue: Record<string, unknown>,
  t: Translate = translateEnglish,
): ToolPresentation {
  const path = stringArgument(argumentsValue, "path");
  switch (name) {
    case "read":
      return tool(t("tool.readFile"), path, "read", "↗");
    case "glob":
      return tool(
        t("tool.findFiles"),
        stringArgument(argumentsValue, "pattern"),
        "read",
        "⌕",
      );
    case "grep": {
      const pattern = stringArgument(argumentsValue, "pattern");
      return tool(
        t("tool.searchText"),
        pattern,
        "read",
        "⌕",
        path ? t("tool.within", { path }) : undefined,
      );
    }
    case "apply_patch":
      return tool(
        t("tool.applyPatch"),
        patchSubject(argumentsValue, t),
        "write",
        "±",
      );
    case "edit":
      return tool(t("tool.editFile"), path, "write", "±");
    case "write":
      return tool(t("tool.writeFile"), path, "write", "+");
    case "shell":
      return tool(
        t("tool.runCommand"),
        stringArgument(argumentsValue, "command"),
        "exec",
        ">_",
      );
	case "read_shell_output":
	  return tool(
		t("tool.readCommandOutput"),
		stringArgument(argumentsValue, "shell_id"),
		"exec",
		">_",
	  );
	case "stop_shell":
	  return tool(
		t("tool.stopCommand"),
		stringArgument(argumentsValue, "shell_id"),
		"exec",
		"■",
	  );
	case "lsp": {
	  const operation = stringArgument(argumentsValue, "operation");
	  return tool(
		operation ? humanize(operation) : t("tool.queryLanguageServer"),
		path ?? stringArgument(argumentsValue, "query"),
		"read",
		"⌘",
	  );
	}
    case "web_search":
      return tool(
        t("tool.searchWeb"),
        stringArgument(argumentsValue, "query"),
        "network",
        "◎",
      );
    case "web_fetch":
    case "http_request":
      return tool(
        t("tool.fetchNetworkResource"),
        stringArgument(argumentsValue, "url"),
        "network",
        "◎",
      );
    case "list_skills":
      return tool(t("tool.discoverSkills"), undefined, "read", "◇");
    case "load_skill":
    case "read_skill_resource":
      return tool(
        t("tool.readSkill"),
        stringArgument(argumentsValue, "name"),
        "read",
        "◇",
      );
    case "propose_skill":
      return tool(
        t("tool.proposeSkill"),
        stringArgument(argumentsValue, "name"),
        "control",
        "◇",
      );
    case "enter_plan_mode":
      return tool(t("tool.enterPlanMode"), undefined, "control", "▤");
    case "set_plan":
      return tool(
        t("tool.updatePlan"),
        listCount(argumentsValue, "steps", t),
        "control",
        "▤",
      );
    case "exit_plan_mode":
      return tool(t("tool.reviewPlan"), undefined, "control", "▤");
    case "create_goal":
      return tool(
        t("tool.createGoal"),
        stringArgument(argumentsValue, "objective"),
        "control",
        "◆",
      );
    case "get_goal":
      return tool(t("tool.inspectGoal"), undefined, "control", "◆");
    case "report_goal_outcome":
      return tool(
        t("tool.settleGoalRun"),
        stringArgument(argumentsValue, "status"),
        "control",
        "◆",
      );
    case "read_tool_result":
      return tool(
        t("tool.continueResult"),
        stringArgument(argumentsValue, "result_id"),
        "read",
        "↗",
      );
    case "delegate_task":
      return tool(
        t("tool.delegateTask"),
        stringArgument(argumentsValue, "summary"),
        "control",
        "⑂",
        stringArgument(argumentsValue, "instructions"),
      );
    default:
      if (name.startsWith("mcp_")) {
        return tool(
          humanize(name.slice(4)),
          primaryArgument(argumentsValue),
          "network",
          "◎",
        );
      }
      return tool(
        name ? humanize(name) : t("tool.defaultName"),
        primaryArgument(argumentsValue),
        "tool",
        "◇",
      );
  }
}

export function toolStatusLabel(
  item: Item,
  t: Translate = translateEnglish,
) {
  if (item.status === "running") return t("tool.status.running");
  if (item.status === "completed") return t("tool.status.complete");
  return item.error?.type === "tool_canceled"
    ? t("tool.status.canceled")
    : t("tool.status.failed");
}

export function formatToolDuration(milliseconds: number) {
  if (milliseconds < 1_000) return `${milliseconds} ms`;
  if (milliseconds < 60_000) {
    return `${(milliseconds / 1_000).toFixed(milliseconds < 10_000 ? 1 : 0)} s`;
  }
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1_000);
  return `${minutes}m ${seconds}s`;
}

export function formatToolValue(value: unknown) {
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2) ?? String(value);
}

export function stringArgument(
  value: Record<string, unknown>,
  key: string,
) {
  const candidate = value[key];
  return typeof candidate === "string" && candidate !== ""
    ? candidate
    : undefined;
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function tool(
  title: string,
  subject: string | undefined,
  kind: ToolPresentation["kind"],
  glyph: string,
  detail?: string,
): ToolPresentation {
  return {
    title,
    kind,
    glyph,
    ...(subject ? { subject } : {}),
    ...(detail ? { detail } : {}),
  };
}

function primaryArgument(value: Record<string, unknown>) {
  for (const key of ["path", "query", "url", "name", "command"]) {
    const candidate = stringArgument(value, key);
    if (candidate) return candidate;
  }
  return undefined;
}

function patchSubject(value: Record<string, unknown>, t: Translate) {
  const patch = stringArgument(value, "patch");
  if (!patch) return undefined;
  const files = [
    ...patch.matchAll(/^\*\*\* (?:Update|Add|Delete) File: (.+)$/gm),
  ];
  if (files.length === 1) return files[0]?.[1];
  if (files.length > 1) return t("tool.fileCount", { count: files.length });
  return t("tool.workspaceChanges");
}

function listCount(
  value: Record<string, unknown>,
  key: string,
  t: Translate,
) {
  const list = value[key];
  return Array.isArray(list)
    ? t("tool.stepCount", { count: list.length })
    : undefined;
}

function humanize(value: string) {
  return value
    .replaceAll("_", " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}
