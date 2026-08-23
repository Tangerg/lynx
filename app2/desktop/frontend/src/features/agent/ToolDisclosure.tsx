import type { ReactNode } from "react";

import type { Item, RunSummary } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
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
  const { t } = useLocalization();
  const tool = item.tool;
  const presentation = presentTool(tool?.name ?? "", tool?.arguments ?? {}, t);
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
          {run?.parentRunId ? <small>{t("tool.delegated")}</small> : null}
          {item.approvalDecision ? (
            <small>
              {item.approvalDecision === "approve"
                ? t("tool.approved")
                : t("tool.denied")}
            </small>
          ) : null}
          {item.durationMillis !== undefined ? (
            <small>{formatToolDuration(item.durationMillis)}</small>
          ) : null}
          <small>{toolStatusLabel(item, t)}</small>
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
          <summary>{t("tool.arguments")}</summary>
          <pre>{formatToolValue(tool?.arguments ?? {})}</pre>
        </details>
        <footer>
          {item.safetyClass ? <span>{item.safetyClass}</span> : null}
          <code>{tool?.name ?? t("tool.unknownIdentity")}</code>
        </footer>
      </div>
    </details>
  );
}

function LiveToolResult({ output }: { output: LiveToolOutput }) {
  const { t } = useLocalization();
  return (
    <section className="tool-result live-tool-result">
      <header>
        <strong>{t("tool.liveOutput")}</strong>
        {output.truncated ? <span>{t("tool.boundedTail")}</span> : null}
      </header>
      <pre>{output.text}</pre>
    </section>
  );
}

function ToolResult({ name, result }: { name: string; result: unknown }) {
  const { t } = useLocalization();
  if (result === undefined) return null;
  if (name === "shell" && isRecord(result)) {
    return (
      <section className="tool-result shell-result">
        <header>
          <strong>{t("tool.commandOutput")}</strong>
          {typeof result.exit_code === "number" ? (
            <span data-failed={result.exit_code !== 0}>
              {t("tool.exitCode", { code: result.exit_code })}
            </span>
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
          <strong>{t("tool.fileContent")}</strong>
          {lineRange(result, t) ? <span>{lineRange(result, t)}</span> : null}
          {result.truncated === true ? <span>{t("tool.truncated")}</span> : null}
        </header>
        <pre>{result.content}</pre>
      </section>
    );
  }
  return (
    <section className="tool-result">
      <header><strong>{t("tool.result")}</strong></header>
      <pre>{formatToolValue(result)}</pre>
    </section>
  );
}

function lineRange(
  value: Record<string, unknown>,
  t: ReturnType<typeof useLocalization>["t"],
) {
  const start = value.start_line;
  const end = value.end_line;
  return typeof start === "number" && typeof end === "number"
    ? t("tool.lineRange", { start, end })
    : undefined;
}
