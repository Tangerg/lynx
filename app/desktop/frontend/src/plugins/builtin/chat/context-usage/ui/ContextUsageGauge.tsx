import { Gauge, Pressable, RichTooltip } from "@/ui";
import { fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { contextUsageReadout } from "../application/contextUsageReadout";

export function ContextUsageGauge() {
  const t = useT();
  const currentRun = useCurrentRootMaterial();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const { data: models = [] } = useModels();
  const servedSelection =
    currentRun.modelSelection ?? sessions?.find((session) => session.id === activeSessionId);
  const servedModel = models.find(
    (model) => model.provider === servedSelection?.provider && model.id === servedSelection?.model,
  );
  const readout = contextUsageReadout(
    currentRun.contextTokens ?? undefined,
    servedModel?.contextWindow,
  );
  if (!readout) return null;

  const label = t("context.usage.aria", { percent: readout.percent });
  const trigger = (
    <Pressable
      aria-label={label}
      className="-mx-1.5 inline-flex size-7 items-center justify-center rounded-control text-fg-muted hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-focus"
    >
      <Gauge value={readout.ratio} label={t("context.usage.aria", { percent: readout.percent })} />
    </Pressable>
  );

  return (
    <RichTooltip
      trigger={trigger}
      side="top"
      sideOffset={4}
      className="w-38 bg-fg px-3 py-2 font-sans text-ui-md leading-snug text-on-fg"
    >
      <div className="flex flex-col gap-0.5 text-center">
        <span className="opacity-60">{t("context.usage.label")}</span>
        <span className={readout.percent >= 50 ? "opacity-60" : undefined}>
          {t(readout.percent >= 50 ? "context.usage.statusFull" : "context.usage.statusLeft", {
            percent: readout.percent,
            remaining: 100 - readout.percent,
          })}
        </span>
        <span>
          {t("context.usage.tooltip", {
            used: fmtTokens(readout.usedTokens),
            window: fmtTokens(readout.windowTokens),
          })}
        </span>
      </div>
    </RichTooltip>
  );
}
