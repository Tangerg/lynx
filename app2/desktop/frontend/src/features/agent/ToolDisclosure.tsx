import type { ReactNode } from "react";

import type { Item, RunSummary } from "@lyra/runtime-contract";

import {
  formatToolDuration,
  formatToolValue,
  isRecord,
  presentTool,
  toolStatusLabel,
} from "./toolPresentation";
import type { LiveToolOutput } from "./agentSessionTypes";

interface ToolDisclosureProps {
  item: Item;
  run?: RunSummary;
  liveOutput?: LiveToolOutput;
	searchMatch?: boolean;
  children?: ReactNode;
}

export function ToolDisclosure({
  item,
  run,
  liveOutput,
	searchMatch,
  children,
}: ToolDisclosureProps) {
  const tool = item.tool;
  const presentation = presentTool(tool?.name ?? "", tool?.arguments ?? {});
  const child = run?.parentRunId !== undefined;
  return (
    <details
      className="narrative-item tool-turn"
      data-child={child}
      data-status={item.status}
	  data-item-id={item.id}
	  data-search-match={searchMatch === true}
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
            <small>{formatToolDuration(item.durationMillis)}</small>
          ) : null}
          <small>{toolStatusLabel(item)}</small>
        </span>
      </summary>
      <div className="tool-material">
        {presentation.detail ? <p>{presentation.detail}</p> : null}
        {children}
        {liveOutput ? <LiveToolResult output={liveOutput} /> : null}
        <ToolResult name={tool?.name ?? ""} result={tool?.result} />
        {item.error?.detail ? (
          <p className="tool-error" role="alert">{item.error.detail}</p>
        ) : null}
        <details className="tool-arguments">
          <summary>Arguments</summary>
          <pre>{formatToolValue(tool?.arguments ?? {})}</pre>
        </details>
        <footer>
          {item.safetyClass ? <span>{item.safetyClass}</span> : null}
          <code>{tool?.name ?? "unknown_tool"}</code>
        </footer>
      </div>
    </details>
  );
}

function LiveToolResult({ output }: { output: LiveToolOutput }) {
  return (
    <section className="tool-result live-tool-result">
      <header>
        <strong>Live output</strong>
        {output.truncated ? <span>bounded tail</span> : null}
      </header>
      <pre>{output.text}</pre>
    </section>
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
      <pre>{formatToolValue(result)}</pre>
    </section>
  );
}

function lineRange(value: Record<string, unknown>) {
  const start = value.start_line;
  const end = value.end_line;
  return typeof start === "number" && typeof end === "number"
    ? `lines ${start}–${end}`
    : undefined;
}
