import type { CommandSpec } from "@/plugins/sdk";
import { lookupCommandOwner, reportPluginError } from "@/plugins/sdk";

export function runPaletteCommand(command: CommandSpec, close: () => void): void {
  close();
  void Promise.resolve(command.run()).catch((error) => {
    console.error(`[plugin] command ${command.id} threw:`, error);
    const owner = lookupCommandOwner(command.id) ?? "unknown";
    reportPluginError(owner, "command", error, `command: ${command.id}`);
  });
}
