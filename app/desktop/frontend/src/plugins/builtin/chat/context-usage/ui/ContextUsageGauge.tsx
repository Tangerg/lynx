import { Gauge, Tooltip } from "@/ui";
import { fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { useSelectedModel } from "@/plugins/builtin/chat/composer/public/selectedModel";
import { contextUsageReadout } from "../application/contextUsageReadout";

/** Past this the window is the thing about to go wrong, so it stops being chrome. */
const CROWDED = 0.9;

export function ContextUsageGauge() {
  const t = useT();
  const metrics = useCurrentRootMaterial().metrics;
  const model = useSelectedModel();
  const readout = contextUsageReadout(metrics?.usage.inputTokens, model?.contextWindow);
  if (!readout) return null;

  return (
    <Tooltip
      label={t("context.usage.tooltip", {
        used: fmtTokens(readout.usedTokens),
        window: fmtTokens(readout.windowTokens),
      })}
    >
      <span
        className={cn(
          "flex items-center",
          readout.ratio >= CROWDED ? "text-warning" : "text-fg-muted",
        )}
      >
        <Gauge
          value={readout.ratio}
          label={t("context.usage.aria", { percent: readout.percent })}
        />
      </span>
    </Tooltip>
  );
}
