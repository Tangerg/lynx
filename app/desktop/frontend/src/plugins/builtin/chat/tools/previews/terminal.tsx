// terminal preview — shell + the background-shell family (run_in_background /
// shell_output / shell_kill), all terminal-style plain text.

import type { ToolPreviewProps } from "@/plugins/sdk";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewFoot } from "@/plugins/builtin/chat/tools/public/previews/PreviewFoot";
import { PreviewPlaceholder } from "@/plugins/builtin/chat/tools/public/previews/PreviewPlaceholder";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";
import { CODE_PANEL } from "./shared";

const MAX_TERM_LINES = 9;

function ShellPreview({ tool, onOpenView }: ToolPreviewProps) {
  // Render THIS call's stdout from `tool.result` — the authoritative merged
  // output reconciled from the completed Item's tool.result.output, with
  // the toolOutput delta stream as the live preview while running (see
  // projections.ts + API.md §4.4.1).
  const output = tool.result?.replace(/\n+$/, "");
  const lines = output ? output.split("\n") : [];
  const hiddenLines = lines.length - MAX_TERM_LINES;
  return (
    <div>
      <div className={cn(CODE_PANEL, "whitespace-pre-wrap break-all")}>
        {lines.length > 0 ? (
          lines.slice(0, MAX_TERM_LINES).map((text, i) => (
            <div key={i}>
              <LinkedText text={text || " "} />
            </div>
          ))
        ) : (
          <PreviewPlaceholder status={tool.status} pending="Running…" idle="(no output)" />
        )}
        {(hiddenLines > 0 || tool.outputTruncated) && (
          <div className="text-fg-faint">
            {hiddenLines > 0 && `… ${hiddenLines} more lines`}
            {tool.outputTruncated && " · output truncated by runtime"}
          </div>
        )}
      </div>
      <PreviewFoot label="tools.preview.openTerminal" onClick={onOpenView} />
    </div>
  );
}

// Previews are keyed by the tool ROUTING KEY = the wire tool `name` (§4.4 /
// §4.4.2 display conventions).
export const shellPreview = definePlugin({
  name: "lyra.builtin.shell",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell" });
    // Background-shell family: all three return terminal-style plain text
    // (start ack / incremental output + status line / kill confirmation).
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "run_in_background" });
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell_output" });
    host.extensions.contribute(TOOL_PREVIEW, ShellPreview, { key: "shell_kill" });
  },
});
