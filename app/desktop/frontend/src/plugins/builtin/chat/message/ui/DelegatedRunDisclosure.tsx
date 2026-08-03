import type { ReactNode } from "react";
import { useState } from "react";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import { IconButton, StatusDot } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
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

  return (
    <AgentActivityDisclosure
      icon="bot"
      // A delegated run produces a transcript of its own, which is the most
      // material anything in this grammar carries.
      shell="card"
      label={model.label}
      detail={
        model.detail ? (
          <span title={model.detail} className="text-pretty">
            {model.detail}
          </span>
        ) : undefined
      }
      trailing={
        <>
          <span
            className={cn(
              "inline-flex items-center gap-1 text-ui-xs font-medium",
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
          <span className="font-mono text-ui-xs tabular-nums text-fg-faint">
            {model.stepsLabel}
          </span>
        </>
      }
      actions={
        <>
          <IconButton
            icon="history"
            size="sm"
            quiet
            title={t("agent.runTree.action.audit")}
            onClick={onOpenAudit}
          />
          {model.cancelable && (
            <IconButton
              icon="stop"
              size="sm"
              quiet
              title={t("agent.runTree.action.cancel")}
              onClick={onCancel}
            />
          )}
        </>
      }
      open={expanded}
      onToggle={() => setPinnedExpanded(!expanded)}
      tone={
        model.status === "error"
          ? "negative"
          : model.status === "waiting" || model.status === "limit"
            ? "warning"
            : "neutral"
      }
      className="my-1.5"
    >
      {hasMaterial ? (
        children
      ) : (
        <p className="text-pretty text-ui-sm text-fg-muted">{t("agent.runTree.material.empty")}</p>
      )}
    </AgentActivityDisclosure>
  );
}
