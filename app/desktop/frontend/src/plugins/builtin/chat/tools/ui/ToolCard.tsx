// Tool activity — one agent tool invocation as a disclosure in the Narrative's
// shared activity grammar. Expands inline to the plugin-contributed preview (or the
// ToolInspector fallback); selected state drives the inspector pane through the
// workspace navigation wiring.
//
// Which shell and which tone the row wears are the MODEL's answers, not this
// component's: a read is a line, something produced is a card, something failed or
// waiting is flagged (see toolActivityShell). Status colour rides the glyph, its
// tray and the metadata — never a wash over the whole row, so nested tools stay
// readable as a hierarchy instead of a stack of competing cards.
import type { IconName } from "@/ui";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { Badge, Icon, IconButton, StatusDot } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import {
  toolCardActions,
  toolCardModel,
  toolCardViewOpener,
  visibleToolMetaItems,
} from "../application/toolCardModel";
import { toolIconFor, toolRoutingKey } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const allActions = useExtensionPoint(TOOL_ACTION);
  const allViewOpeners = useExtensionPoint(TOOL_VIEW_OPENER);
  const actions = toolCardActions(tool, allActions);
  const viewOpener = toolCardViewOpener(tool, allViewOpeners);
  const onOpenView = viewOpener
    ? () => {
        void Promise.resolve(viewOpener.open(tool)).catch((err) => {
          const owner = lookupToolViewOpenerOwner(viewOpener.id) ?? "unknown";
          console.error(`[plugin] tool view opener ${viewOpener.id} threw:`, err);
          reportPluginError(owner, "command", err, `tool view opener: ${viewOpener.id}`);
        });
      }
    : undefined;

  return (
    <AgentActivityDisclosure
      icon={toolRowIcon(tool)}
      tone={model.tone}
      shell={model.shell}
      label={<span title={model.intent.label}>{model.intent.label}</span>}
      detail={
        model.detail ? (
          // A path, a pattern, a command — data, so it takes the technical face.
          <span title={model.detail} className={cn("font-mono", model.isError && "text-negative")}>
            {model.detail}
          </span>
        ) : undefined
      }
      trailing={
        <>
          <ToolMeta items={model.metaItems} running={model.running} />
          {model.running && <StatusDot tone="running" />}
          {/* A refused call is not a finished one. Its glyph says so at 12px, which
              is not a size anyone reads while scrolling — so the state says it in a
              word, the way risk and scope already do on an approval. */}
          {model.denied && <Badge tone="warning">{t("tool.state.denied")}</Badge>}
          {/* A settled call ends with its verdict. Where the call reported nothing
              to count, the tick IS the verdict; where it did, the counts above are,
              and a tick after them is one more identical glyph in a column of them. */}
          {model.showSettledMark && <Icon name="check" size="xs" className="text-success" />}
        </>
      }
      actions={actions.map((action) => (
        <IconButton
          key={action.id}
          icon={action.icon as IconName}
          size="xs"
          quiet={!model.isError}
          title={t(action.title)}
          onClick={(event) => {
            event.stopPropagation();
            void Promise.resolve(action.run(tool)).catch((err) => {
              const owner = lookupToolActionOwner(action.id) ?? "unknown";
              console.error(`[plugin] tool action ${action.id} threw:`, err);
              reportPluginError(owner, "command", err, `tool action: ${action.id}`);
            });
          }}
          className={cn(
            !model.isError &&
              "opacity-0 transition-opacity group-hover/activity:opacity-100 focus-visible:opacity-100",
          )}
        />
      ))}
      open={expanded}
      onToggle={onToggleExpand}
    >
      <ToolPreview tool={tool} onOpenView={onOpenView} />
    </AgentActivityDisclosure>
  );
}

function toolRowIcon(tool: ToolCall): IconName {
  if (tool.status === "err") return "x";
  if (tool.status === "requires-action") return "alert";
  if (tool.status === "denied") return "stop";
  return toolIconFor(toolRoutingKey(tool));
}

function ToolMeta({ items, running }: { items: ToolMetaItem[]; running: boolean }) {
  const shown = visibleToolMetaItems(items, running);
  if (shown.length === 0) return null;

  return (
    <span className="hidden shrink-0 items-center gap-1.5 sm:flex">
      {shown.map((item) => (
        <span
          key={item.id}
          className={cn(
            "font-mono text-ui-xs font-medium tabular-nums",
            toolMetaToneClass(item.tone),
          )}
        >
          {item.label}
        </span>
      ))}
    </span>
  );
}

function toolMetaToneClass(tone: ToolMetaItem["tone"]): string {
  if (tone === "success") return "text-success";
  if (tone === "negative") return "text-negative";
  return "text-fg-muted";
}
