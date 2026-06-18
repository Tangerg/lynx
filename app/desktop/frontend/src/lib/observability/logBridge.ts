// host.log → OpenTelemetry logs bridge. Mirror of the backend's otelslog
// bridge (slog.Default() → LoggerProvider): a frontend log call becomes an
// OTel LogRecord, so logs are the third pillar — correlated with the active
// span (the SDK fills trace_id / span_id from context.active() at emit time)
// and backend-swappable (the same record goes to the local sink in dev and
// OTLP in prod, zero call-site change).
//
// No provider installed yet (before setupObservability runs, or in tests) →
// logs.getLogger returns a no-op logger and emit() is a cheap no-op.
//
// host.log is NOT a hot path (CLAUDE.md §5 forbids per-event logging), so
// bridging every call is fine — no batching needed here (the sink batches).

import { logs, SeverityNumber } from "@opentelemetry/api-logs";

type LogLevel = "debug" | "info" | "warn" | "error";

const SEVERITY: Record<LogLevel, { number: SeverityNumber; text: string }> = {
  debug: { number: SeverityNumber.DEBUG, text: "DEBUG" },
  info: { number: SeverityNumber.INFO, text: "INFO" },
  warn: { number: SeverityNumber.WARN, text: "WARN" },
  error: { number: SeverityNumber.ERROR, text: "ERROR" },
};

const LOGGER_NAME = "lyra-frontend";

/** Emit one frontend log line as an OTel LogRecord, attributed to `scope`
 *  (the plugin/kernel name). The active span's trace context is attached
 *  natively by the SDK. */
export function emitLog(scope: string, level: LogLevel, args: unknown[]): void {
  const sev = SEVERITY[level];
  logs.getLogger(LOGGER_NAME).emit({
    severityNumber: sev.number,
    severityText: sev.text,
    body: args.map(stringify).join(" "),
    attributes: { "scope.name": scope },
  });
}

function stringify(value: unknown): string {
  if (typeof value === "string") return value;
  if (value instanceof Error) return value.stack ?? `${value.name}: ${value.message}`;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
