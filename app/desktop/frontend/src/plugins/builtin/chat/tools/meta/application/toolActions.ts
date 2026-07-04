import type { ToolActionSpec } from "@/plugins/sdk";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";

export interface CopyToolArgsActionOptions {
  title: string;
  copyText: (text: string) => unknown | Promise<unknown>;
}

export function hasCopyableToolArgs(tool: Pick<ToolCall, "args">): boolean {
  return tool.args.trim().length > 0;
}

export function copyToolArgsAction(options: CopyToolArgsActionOptions): ToolActionSpec {
  return {
    id: "copy-args",
    icon: "copy",
    title: options.title,
    order: 0,
    predicate: hasCopyableToolArgs,
    run: async (tool) => {
      await options.copyText(tool.args);
    },
  };
}
