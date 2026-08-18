import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { Icon, Pressable } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { toolCardModel, visibleToolMetaItems } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

/**
 * One call inside a group, as a FLAT row.
 *
 * Members have no independent disclosure state, so they omit nested disclosure
 * chrome and the list gets a rule between
 * rows instead. The one identity mark remains: it is the visual contract that a
 * read, search and language query do not collapse into indistinguishable text.
 *
 * Clicking still opens the call's own preview, underneath, at the row's text column —
 * the capability is unchanged, it just is not advertised by a permanent arrow on every
 * row of a list where every row has one.
 */
export function ToolGroupMember({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const meta = visibleToolMetaItems(model.metaItems, model.running);

  return (
    <div>
      <Pressable
        data-tool={tool.name}
        data-status={tool.status}
        type="button"
        aria-expanded={expanded}
        onClick={onToggleExpand}
        className={cn(
          "flex w-full min-w-0 items-baseline gap-2 px-2 py-1 text-left",
          "transition-colors duration-[var(--dur-color)] hover:bg-hover",
          expanded && "bg-selected",
        )}
      >
        <Icon
          name={toolCallIconFor(tool)}
          size="xs"
          className={cn("shrink-0", model.isError ? "text-negative" : "text-fg-faint")}
        />
        <ToolText
          value={model.detail ?? model.intent.label}
          className={cn(
            "min-w-0 flex-1 text-ui-sm",
            model.isError ? "text-negative" : "text-fg-muted",
          )}
        />
        {meta.length > 0 && (
          <span className="shrink-0 font-mono text-ui-2xs text-fg-faint tabular-nums">
            {meta[meta.length - 1]!.label}
          </span>
        )}
      </Pressable>
      {expanded && (
        <div className="px-2 pb-2">
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}
