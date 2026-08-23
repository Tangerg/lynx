import type { Item } from "@lyra/runtime-contract";

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
): ToolPresentation {
  const path = stringArgument(argumentsValue, "path");
  switch (name) {
    case "read":
      return tool("Read file", path, "read", "↗");
    case "glob":
      return tool(
        "Find files",
        stringArgument(argumentsValue, "pattern"),
        "read",
        "⌕",
      );
    case "grep": {
      const pattern = stringArgument(argumentsValue, "pattern");
      return tool(
        "Search text",
        pattern,
        "read",
        "⌕",
        path ? `Within ${path}` : undefined,
      );
    }
    case "apply_patch":
      return tool("Apply patch", patchSubject(argumentsValue), "write", "±");
    case "edit":
      return tool("Edit file", path, "write", "±");
    case "write":
      return tool("Write file", path, "write", "+");
    case "shell":
      return tool(
        "Run command",
        stringArgument(argumentsValue, "command"),
        "exec",
        ">_",
      );
    case "web_search":
      return tool(
        "Search the web",
        stringArgument(argumentsValue, "query"),
        "network",
        "◎",
      );
    case "web_fetch":
    case "http_request":
      return tool(
        "Fetch network resource",
        stringArgument(argumentsValue, "url"),
        "network",
        "◎",
      );
    case "list_skills":
      return tool("Discover skills", undefined, "read", "◇");
    case "load_skill":
    case "read_skill_resource":
      return tool(
        "Read skill",
        stringArgument(argumentsValue, "name"),
        "read",
        "◇",
      );
    case "propose_skill":
      return tool(
        "Propose skill",
        stringArgument(argumentsValue, "name"),
        "control",
        "◇",
      );
    case "enter_plan_mode":
      return tool("Enter Plan mode", undefined, "control", "▤");
    case "set_plan":
      return tool(
        "Update Plan",
        listCount(argumentsValue, "steps"),
        "control",
        "▤",
      );
    case "exit_plan_mode":
      return tool("Review Plan", undefined, "control", "▤");
    case "create_goal":
      return tool(
        "Create goal",
        stringArgument(argumentsValue, "objective"),
        "control",
        "◆",
      );
    case "get_goal":
      return tool("Inspect goal", undefined, "control", "◆");
    case "report_goal_outcome":
      return tool(
        "Settle goal run",
        stringArgument(argumentsValue, "status"),
        "control",
        "◆",
      );
    case "read_tool_result":
      return tool(
        "Continue tool result",
        stringArgument(argumentsValue, "result_id"),
        "read",
        "↗",
      );
    case "delegate_task":
      return tool(
        "Delegate task",
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
        humanize(name || "Tool"),
        primaryArgument(argumentsValue),
        "tool",
        "◇",
      );
  }
}

export function toolStatusLabel(item: Item) {
  if (item.status === "running") return "running";
  if (item.status === "completed") return "complete";
  return item.error?.type === "tool_canceled" ? "canceled" : "failed";
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

function patchSubject(value: Record<string, unknown>) {
  const patch = stringArgument(value, "patch");
  if (!patch) return undefined;
  const files = [
    ...patch.matchAll(/^\*\*\* (?:Update|Add|Delete) File: (.+)$/gm),
  ];
  if (files.length === 1) return files[0]?.[1];
  if (files.length > 1) return `${files.length} files`;
  return "Workspace changes";
}

function listCount(value: Record<string, unknown>, key: string) {
  const list = value[key];
  return Array.isArray(list) ? `${list.length} steps` : undefined;
}

function humanize(value: string) {
  return value
    .replaceAll("_", " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}
