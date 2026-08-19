import { Gauge, RichTooltip } from "@/ui";
import { fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId, useAgentSessions } from "@/plugins/builtin/agent/public/session";
import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { contextUsageReadout } from "../application/contextUsageReadout";

export function ContextUsageGauge() {
  const t = useT();
  const material = useCurrentRootMaterial();
  const activeSessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const { data: models = [] } = useModels();
  const servedModelId = sessions?.find((session) => session.id === activeSessionId)?.model;
  const servedModel = models.find((model) => model.id === servedModelId);
  const readout = contextUsageReadout(
    material.contextTokens ?? undefined,
    servedModel?.contextWindow,
  );
  if (!readout) return null;

  const trigger = (
    <span className="inline-flex items-center justify-center text-fg-muted">
      <Gauge value={readout.ratio} label={t("context.usage.aria", { percent: readout.percent })} />
    </span>
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
