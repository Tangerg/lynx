import type { LogLevel } from "./types";
import { emitLog as emitOtelLog } from "@/lib/observability/logBridge";
import { safeCall } from "./errors";
import { LOG_SUBSCRIBER } from "./kernelPoints";
import { lookupExtensionPoint } from "./selectors/extensions";

// Method-name lookup beats nested ternaries — adding a level is one line.
// Stored as the method name (not a reference) so vitest's `vi.spyOn(console,
// "info")` after module load still binds.
const CONSOLE_METHOD: Record<LogLevel, "log" | "info" | "warn" | "error"> = {
  debug: "log",
  info: "info",
  warn: "warn",
  error: "error",
};

export function logToConsole(plugin: string, level: LogLevel, args: unknown[]): void {
  console[CONSOLE_METHOD[level]](`[plugin:${plugin}]`, ...args);
}

export function emitPluginLog(plugin: string, level: LogLevel, args: unknown[]): void {
  logToConsole(plugin, level, args);
  // Third pillar: mirror the line into OTel logs (no-op until a LoggerProvider
  // is installed). Correlated with the active span by the SDK.
  emitOtelLog(plugin, level, args);
  const event = { plugin, level, args, timestamp: Date.now() };
  for (const fn of lookupExtensionPoint(LOG_SUBSCRIBER)) {
    safeCall(() => fn(event), "[plugin] log subscriber threw:");
  }
}
