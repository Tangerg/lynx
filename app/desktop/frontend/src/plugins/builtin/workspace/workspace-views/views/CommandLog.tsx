import type { ToolCall } from "@/protocol/run/viewState";
import { useT } from "@/lib/i18n";

// Consolidated command log (G5): the agent's command executions for the active
// session. Output streams in via item.delta{toolOutput} → item.completed and is
// landed in the run fold's toolCalls — a running command tails live, a finished
// one keeps its full output + exit code.
export function CommandLog({ commands }: { commands: ToolCall[] }) {
  const t = useT();
  return (
    <div className="flex flex-col gap-3 px-3 py-3 font-mono text-[12px] leading-relaxed">
      {commands.map((c) => (
        <div key={c.id}>
          <div className="flex items-baseline gap-2">
            <span className="shrink-0 text-fg-faint">$</span>
            <span className="min-w-0 break-all text-fg">{c.fn}</span>
            {c.status === "running" && (
              <span className="shrink-0 text-accent">{t("commandLog.running")}</span>
            )}
            {c.status === "err" && (
              <span className="shrink-0 text-negative">{t("commandLog.failed")}</span>
            )}
            {c.exitCode !== undefined && c.exitCode !== 0 && (
              <span className="shrink-0 text-negative">
                {t("commandLog.exit", { code: c.exitCode })}
              </span>
            )}
          </div>
          {c.result ? (
            <pre className="mt-1 whitespace-pre-wrap break-words text-fg-muted">
              {c.result}
              {c.outputTruncated ? `\n${t("commandLog.truncated")}` : ""}
            </pre>
          ) : null}
        </div>
      ))}
    </div>
  );
}
