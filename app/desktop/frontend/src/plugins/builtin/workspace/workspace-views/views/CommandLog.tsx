import { memo } from "react";
import { useT } from "@/lib/i18n";
import type { WorkspaceCommandActivity } from "@/plugins/builtin/workspace/application/toolActivity";

// Consolidated command log (G5): the agent's command executions for the active
// session. Output streams in via item.delta{toolOutput} → item.completed and is
// landed in the run fold's toolCalls — a running command tails live, a finished
// one keeps its full output + exit code.
export const CommandLog = memo(function CommandLog({
  commands,
}: {
  commands: readonly WorkspaceCommandActivity[];
}) {
  const t = useT();
  return (
    <div className="flex flex-col gap-2.5 px-3 py-3 font-mono text-[12px] leading-relaxed">
      {commands.map((c) => (
        <div key={c.id} className="rounded-[10px] bg-surface-2 px-3 py-2.5">
          <div className="flex items-baseline gap-2">
            <span className="shrink-0 text-fg-faint">$</span>
            <span className="min-w-0 truncate text-fg" title={c.command}>
              {c.command}
            </span>
            {c.status === "running" && (
              <span className="shrink-0 text-accent">{t("commandLog.running")}</span>
            )}
            {c.status === "failed" && (
              <span className="shrink-0 text-negative">{t("commandLog.failed")}</span>
            )}
            {c.exitCode !== undefined && c.exitCode !== 0 && (
              <span className="shrink-0 text-negative">
                {t("commandLog.exit", { code: c.exitCode })}
              </span>
            )}
          </div>
          {c.output ? (
            <pre className="mt-1.5 whitespace-pre-wrap break-words text-fg-muted">
              {c.output}
              {c.outputTruncated ? `\n${t("commandLog.truncated")}` : ""}
            </pre>
          ) : null}
        </div>
      ))}
    </div>
  );
});
