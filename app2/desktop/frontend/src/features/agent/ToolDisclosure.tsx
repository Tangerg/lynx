import type { ReactNode } from "react";

import type { Item, RunRef } from "@lyra/runtime-contract";

interface ToolDisclosureProps {
  item: Item;
  run?: RunRef;
  children?: ReactNode;
}

export function ToolDisclosure({ item, run, children }: ToolDisclosureProps) {
  const tool = item.tool;
  const presentation = presentTool(tool?.name ?? "", tool?.arguments ?? {});
  const child = run?.parentRunId !== undefined;
  return (
    <details
      className="narrative-item tool-turn"
      data-child={child}
      data-status={item.status}
      defaultOpen={item.status === "running"}
    >
      <summary>
        <span className="tool-mark" data-kind={presentation.kind} aria-hidden="true">
          {presentation.glyph}
        </span>
        <span className="tool-heading">
          <strong>{presentation.title}</strong>
          {presentation.subject ? <small>{presentation.subject}</small> : null}
        </span>
        <span className="tool-facts">
          {run?.parentRunId ? <small>delegated</small> : null}
          {item.approvalDecision ? (
            <small>{item.approvalDecision === "approve" ? "approved" : "denied"}</small>
          ) : null}
          {item.durationMillis !== undefined ? (
            <small>{formatDuration(item.durationMillis)}</small>
          ) : null}
          <small>{statusLabel(item)}</small>
        </span>
      </summary>
      <div className="tool-material">
        {presentation.detail ? <p>{presentation.detail}</p> : null}
        {children}
        <ToolResult name={tool?.name ?? ""} result={tool?.result} />
        {item.error?.detail ? (
          <p className="tool-error" role="alert">{item.error.detail}</p>
        ) : null}
        <details className="tool-arguments">
          <summary>Arguments</summary>
          <pre>{formatValue(tool?.arguments ?? {})}</pre>
        </details>
        <footer>
          {item.safetyClass ? <span>{item.safetyClass}</span> : null}
          <code>{tool?.name ?? "unknown_tool"}</code>
        </footer>
      </div>
    </details>
  );
}

function ToolResult({ name, result }: { name: string; result: unknown }) {
  if (result === undefined) return null;
  if (name === "shell" && isRecord(result)) {
    return (
      <section className="tool-result shell-result">
        <header>
          <strong>Command output</strong>
          {typeof result.exit_code === "number" ? (
            <span data-failed={result.exit_code !== 0}>exit {result.exit_code}</span>
          ) : null}
          {typeof result.duration === "string" ? <span>{result.duration}</span> : null}
        </header>
        {typeof result.stdout === "string" && result.stdout !== "" ? (
          <pre>{result.stdout}</pre>
        ) : null}
        {typeof result.stderr === "string" && result.stderr !== "" ? (
          <pre className="stderr">{result.stderr}</pre>
        ) : null}
      </section>
    );
  }
  if (name === "read" && isRecord(result) && typeof result.content === "string") {
    return (
      <section className="tool-result file-result">
        <header>
          <strong>File content</strong>
          {lineRange(result) ? <span>{lineRange(result)}</span> : null}
          {result.truncated === true ? <span>truncated</span> : null}
        </header>
        <pre>{result.content}</pre>
      </section>
    );
  }
  return (
    <section className="tool-result">
      <header><strong>Result</strong></header>
      <pre>{formatValue(result)}</pre>
    </section>
  );
}

interface ToolPresentation {
  title: string;
  subject?: string;
  detail?: string;
  kind: "read" | "write" | "exec" | "network" | "control" | "tool";
  glyph: string;
}

function presentTool(
  name: string,
  argumentsValue: Record<string, unknown>,
): ToolPresentation {
  const path = stringArgument(argumentsValue, "path");
  switch (name) {
    case "read":
      return tool("Read file", path, "read", "↗");
    case "glob":
      return tool("Find files", stringArgument(argumentsValue, "pattern"), "read", "⌕");
    case "grep": {
      const pattern = stringArgument(argumentsValue, "pattern");
      return tool("Search text", pattern, "read", "⌕", path ? `Within ${path}` : undefined);
    }
    case "apply_patch":
      return tool("Apply patch", patchSubject(argumentsValue), "write", "±");
    case "edit":
      return tool("Edit file", path, "write", "±");
    case "write":
      return tool("Write file", path, "write", "+");
    case "shell":
      return tool("Run command", stringArgument(argumentsValue, "command"), "exec", ">_");
    case "web_search":
      return tool("Search the web", stringArgument(argumentsValue, "query"), "network", "◎");
    case "web_fetch":
    case "http_request":
      return tool("Fetch network resource", stringArgument(argumentsValue, "url"), "network", "◎");
    case "list_skills":
      return tool("Discover skills", undefined, "read", "◇");
    case "load_skill":
    case "read_skill_resource":
      return tool("Read skill", stringArgument(argumentsValue, "name"), "read", "◇");
    case "enter_plan_mode":
      return tool("Enter Plan mode", undefined, "control", "▤");
    case "set_plan":
      return tool("Update Plan", listCount(argumentsValue, "steps"), "control", "▤");
    case "exit_plan_mode":
      return tool("Review Plan", undefined, "control", "▤");
    case "create_goal":
      return tool("Create goal", stringArgument(argumentsValue, "objective"), "control", "◆");
    case "get_goal":
      return tool("Inspect goal", undefined, "control", "◆");
    case "report_goal_outcome":
      return tool("Settle goal run", stringArgument(argumentsValue, "status"), "control", "◆");
    case "read_tool_result":
      return tool("Continue tool result", stringArgument(argumentsValue, "result_id"), "read", "↗");
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
        return tool(humanize(name.slice(4)), primaryArgument(argumentsValue), "network", "◎");
      }
      return tool(humanize(name || "Tool"), primaryArgument(argumentsValue), "tool", "◇");
  }
}

function tool(
  title: string,
  subject: string | undefined,
  kind: ToolPresentation["kind"],
  glyph: string,
  detail?: string,
): ToolPresentation {
  return { title, kind, glyph, ...(subject ? { subject } : {}), ...(detail ? { detail } : {}) };
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
  const files = [...patch.matchAll(/^\*\*\* (?:Update|Add|Delete) File: (.+)$/gm)];
  if (files.length === 1) return files[0]?.[1];
  if (files.length > 1) return `${files.length} files`;
  return "Workspace changes";
}

function listCount(value: Record<string, unknown>, key: string) {
  const list = value[key];
  return Array.isArray(list) ? `${list.length} steps` : undefined;
}

function stringArgument(value: Record<string, unknown>, key: string) {
  const candidate = value[key];
  return typeof candidate === "string" && candidate !== "" ? candidate : undefined;
}

function humanize(value: string) {
  return value
    .replaceAll("_", " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function statusLabel(item: Item) {
  if (item.status === "running") return "running";
  if (item.status === "completed") return "complete";
  return item.error?.type === "tool_canceled" ? "canceled" : "failed";
}

function formatDuration(milliseconds: number) {
  if (milliseconds < 1_000) return `${milliseconds} ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(milliseconds < 10_000 ? 1 : 0)} s`;
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1_000);
  return `${minutes}m ${seconds}s`;
}

function formatValue(value: unknown) {
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2) ?? String(value);
}

function lineRange(value: Record<string, unknown>) {
  const start = value.start_line;
  const end = value.end_line;
  return typeof start === "number" && typeof end === "number"
    ? `lines ${start}–${end}`
    : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
