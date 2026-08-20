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
 * Members have no independent disclosure chrome. The one identity mark remains:
 * it is the visual contract that a read, search and language query do not collapse
 * into indistinguishable text.
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
          "flex w-full min-w-0 items-baseline gap-1.5 py-0.5 text-left text-fg-muted",
          "hover:text-fg",
          expanded && "text-fg",
        )}
      >
        <Icon name={toolCallIconFor(tool)} size="xs" className="shrink-0 text-fg-faint" />
        <ToolText
          value={model.detail ?? model.intent.label}
          className="min-w-0 flex-1 text-ui-sm text-inherit"
        />
        {meta.length > 0 && (
          <span className="shrink-0 font-mono text-ui-2xs text-fg-faint tabular-nums">
            {meta[meta.length - 1]!.label}
          </span>
        )}
      </Pressable>
      {expanded && (
        <div className="pt-1.5 pb-1.5">
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}
