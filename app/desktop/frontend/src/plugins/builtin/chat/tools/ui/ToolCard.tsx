// Tool activity — one agent tool invocation as a disclosure in the Narrative's
// shared activity grammar. Expands inline to the plugin-contributed preview (or the
// ToolInspector fallback); selected state drives the inspector pane through the
// workspace navigation wiring.
//
// Which shell and tone the row wears are the MODEL's answers, not this component's.
// Every invocation stays on the same transparent Codex work-narrative plane; the
// disclosed terminal, diff or structured result is the material surface.
import type { IconName } from "@/ui";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { DiffStat, IconButton, StatusDot } from "@/ui";
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
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";

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
      data-tool={tool.name}
      data-status={tool.status}
      icon={toolCallIconFor(tool)}
      tone={model.tone}
      shell={model.shell}
      label={<ToolText value={model.intent.label} className="w-full" />}
      detail={
        model.detail ? (
          // A path, a pattern, a command — data, so it takes the technical face.
          <ToolText value={model.detail} className="w-full font-mono" />
        ) : undefined
      }
      trailing={
        <>
          {/* Before the counts, because it is the count a reader of an edit came
              for — and the same `+n −m` the diff header and run summary show. */}
          {model.diffStat && (
            <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
          )}
          <ToolMeta items={model.metaItems} running={model.running} />
          {model.running && <StatusDot tone="running" />}
          {/* Refusal remains explicit, but as one quiet status word in the same
              narrative line — not a yellow capsule that promotes the whole call
              into a second approval surface. */}
          {model.denied && (
            <span data-slot="tool-status" className="font-sans text-ui-xs text-fg-muted">
              {t("tool.state.denied")}
            </span>
          )}
        </>
      }
      actions={actions.map((action) => (
        <IconButton
          key={action.id}
          icon={action.icon as IconName}
          size="xs"
          quiet
          title={t(action.title)}
          onClick={(event) => {
            event.stopPropagation();
            void Promise.resolve(action.run(tool)).catch((err) => {
              const owner = lookupToolActionOwner(action.id) ?? "unknown";
              console.error(`[plugin] tool action ${action.id} threw:`, err);
              reportPluginError(owner, "command", err, `tool action: ${action.id}`);
            });
          }}
          className="opacity-0 transition-opacity group-hover/activity-header:opacity-100 focus-visible:opacity-100"
        />
      ))}
      open={expanded}
      onToggle={onToggleExpand}
    >
      <ToolPreview tool={tool} onOpenView={onOpenView} />
    </AgentActivityDisclosure>
  );
}

function ToolMeta({ items, running }: { items: ToolMetaItem[]; running: boolean }) {
  const shown = visibleToolMetaItems(items, running);
  if (shown.length === 0) return null;

  return (
    // Against the TRANSCRIPT's width, not the window's. `sm:` asked the viewport
    // whether this row has room, which it cannot know: the dock and the drawer take
    // their width from the same card this row sits in, so a wide window can hold a
    // narrow transcript and a cramped row would keep its chips. The pane declares
    // itself a container (ChatStream); this asks that.
    <span className="hidden shrink-0 items-center gap-1.5 @sm:flex">
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
  // An exit code is already an exact verdict. Codex keeps it in secondary ink;
  // painting it red would make the metadata louder than the command it explains.
  return tone === "success" ? "text-success" : "text-fg-muted";
}
