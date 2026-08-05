// One run of the agent's own work, folded.
//
// A turn alternates between doing and answering. Everything it did before an answer is
// the account of how that answer was reached — worth keeping, not worth reading first —
// so it collapses to a single row and the transcript reads as work · answer · work ·
// answer. Open it and the members are exactly the rows that would have been there
// inline; close it again and they go back.
//
// The `line` shell, because a fold over process is process: a card here would make the
// thing being hidden heavier than the answer it sits above.

import { useState, type ReactNode } from "react";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import { waveStepCount } from "@/plugins/builtin/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { unitSeamClass } from "../application/renderUnitRhythm";
import type { BlockCtx } from "./blockContext";
import { waveGlyphs } from "./narrativeWaveGlyphs";

interface Props {
  units: MessageRenderUnit[];
  ctx: BlockCtx;
  /** The transcript's own unit dispatcher, injected rather than imported: a fold is
   *  one CASE of that dispatch, so importing it back would close a cycle. Same
   *  arrangement DelegatedNarrative uses for the same reason. */
  renderUnit: (unit: MessageRenderUnit, ctx: BlockCtx) => ReactNode;
}

export function NarrativeWave({ units, ctx, renderUnit }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const glyphs = waveGlyphs(units, ctx.toolCalls);

  return (
    <AgentActivityDisclosure
      shell="line"
      // What kind of round this was — the work that dominated it. One glyph, because
      // the gutter is one slot wide for every row in the transcript.
      icon={glyphs[0] ?? "sparkle"}
      // …and the rest of what is inside, at the other end of the row. These used to be
      // the leading mark, which is where they did damage: a strip is as wide as its
      // glyph count, so it pushed this row's label off the column every other row's
      // label sits on, and a long enough one ran straight into it. They say the same
      // thing here, in a slot that may be any width.
      trailing={
        glyphs.length > 1 ? (
          <span className="flex items-center gap-1">
            {glyphs.slice(1).map((glyph, index) => (
              <Icon key={index} name={glyph} size="xs" />
            ))}
          </span>
        ) : undefined
      }
      label={t("agent.steps", { count: waveStepCount(units) })}
      open={open}
      onToggle={() => setOpen((value) => !value)}
      // A wave holds a whole round of work — reasoning plus every tool call in
      // it — so it is routinely taller than the reading column. Scrolling its
      // rows used to carry the count away with it, leaving a stack of tool rows
      // with nothing saying what round they belonged to.
      stickyHeader
    >
      {/* Each member already knows it is superseded — this wave exists BECAUSE an
          answer followed it — so nothing inside springs open when the wave does.
          Same seam owner as the transcript: a fold holds the rows it would otherwise
          have shown inline, so it has to space them the same way. */}
      {units.map((unit, index) => (
        <div key={index} className={unitSeamClass(units[index - 1], unit)}>
          {renderUnit(unit, ctx)}
        </div>
      ))}
    </AgentActivityDisclosure>
  );
}
