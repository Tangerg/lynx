import type { ReactNode } from "react";
import { useId, useState } from "react";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import { Collapsible, Icon, IconButton, StatusDot } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { delegatedRunCardModel } from "../application/delegatedRunCardModel";

interface Props {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
  hasMaterial: boolean;
  onCancel: () => void;
  onOpenAudit: () => void;
  children: ReactNode;
}

export function DelegatedRunDisclosure({
  run,
  ordinal,
  siblingCount,
  hasMaterial,
  onCancel,
  onOpenAudit,
  children,
}: Props) {
  const t = useT();
  const model = delegatedRunCardModel(t, run, ordinal, siblingCount);
  const [pinnedExpanded, setPinnedExpanded] = useState<boolean | null>(null);
  const expanded = pinnedExpanded ?? model.autoExpanded;
  const headingId = useId();
  const panelId = useId();

  return (
    <section className="my-2 overflow-hidden rounded-lg border border-field bg-surface">
      <div className="flex min-h-10 items-stretch">
        <button
          id={headingId}
          type="button"
          aria-expanded={expanded}
          aria-controls={panelId}
          onClick={() => setPinnedExpanded(!expanded)}
          className="flex min-h-10 min-w-0 flex-1 items-center gap-2.5 px-3 text-left transition-colors duration-100 hover:bg-hover"
        >
          <span className="grid h-7 w-7 shrink-0 place-items-center rounded-sm bg-surface-2 text-fg-muted">
            <Icon name="bot" size={15} />
          </span>
          <span className="min-w-0 flex-1 py-2">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-ui-md font-semibold text-fg">{model.label}</span>
              <span
                className={cn(
                  "inline-flex shrink-0 items-center gap-1.5 text-ui-xs font-medium",
                  model.status === "running"
                    ? "text-accent"
                    : model.status === "waiting" || model.status === "limit"
                      ? "text-warning"
                      : model.status === "error"
                        ? "text-negative"
                        : "text-fg-muted",
                )}
              >
                <StatusDot tone={model.dotTone} />
                {model.statusLabel}
              </span>
            </span>
            <span className="mt-0.5 flex min-w-0 items-center gap-2 text-ui-sm text-fg-muted">
              {model.detail && (
                <span title={model.detail} className="truncate text-pretty">
                  {model.detail}
                </span>
              )}
              <span className="ml-auto shrink-0 font-mono tabular-nums">{model.stepsLabel}</span>
            </span>
          </span>
          <Icon
            name="chevron-down"
            size={14}
            className={cn(
              "shrink-0 text-fg-faint transition-transform duration-150 motion-reduce:transition-none",
              !expanded && "-rotate-90",
            )}
          />
        </button>

        <div className="flex shrink-0 items-center border-l border-field px-0.5">
          <IconButton
            icon="history"
            size="lg"
            quiet
            title={t("agent.runTree.action.audit")}
            onClick={onOpenAudit}
          />
          {model.cancelable && (
            <IconButton
              icon="stop"
              size="lg"
              quiet
              title={t("agent.runTree.action.cancel")}
              onClick={onCancel}
            />
          )}
        </div>
      </div>

      <Collapsible open={expanded}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={headingId}
          className="border-t border-field px-3 pb-3 pt-2.5"
        >
          {hasMaterial ? (
            children
          ) : (
            <p className="text-pretty text-ui-sm text-fg-muted">
              {t("agent.runTree.material.empty")}
            </p>
          )}
        </div>
      </Collapsible>
    </section>
  );
}
