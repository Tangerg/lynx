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
import {
  summarizeActivity,
  waveStepCount,
  waveToolCalls,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type { TurnFacts } from "@/plugins/builtin/agent/public/conversation";
import { unitSeamClass } from "../application/renderUnitRhythm";
import type { BlockCtx } from "./blockContext";
import { waveGlyph } from "./narrativeWaveGlyphs";

interface Props {
  units: MessageRenderUnit[];
  facts: TurnFacts;
  ctx: BlockCtx;
  /** The transcript's own unit dispatcher, injected rather than imported: a fold is
   *  one CASE of that dispatch, so importing it back would close a cycle. Same
   *  arrangement DelegatedNarrative uses for the same reason. */
  renderUnit: (unit: MessageRenderUnit, facts: TurnFacts, ctx: BlockCtx) => ReactNode;
}

export function NarrativeWave({ units, facts, ctx, renderUnit }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const tools = waveToolCalls(units, facts.toolCalls);
  // What the round DID, in the same tally a tool group uses, with the total behind
  // the row in the meta column. A row that only counted its steps ("6 steps") asked
  // the reader to open it to learn whether anything had been changed or run — which
  // is the one thing a folded account of past work has to be able to say closed.
  // A round of pure thinking has no acts to tally, so the count carries it alone.
  const summary = summarizeActivity(t, tools);
  const steps = t("agent.steps", { count: waveStepCount(units) });

  return (
    <AgentActivityDisclosure
      shell="line"
      // What kind of round this was — the act it opened with. One glyph, because
      // the gutter is one slot wide for every row in the transcript.
      icon={waveGlyph(units, facts.toolCalls) ?? "sparkle"}
      label={summary || steps}
      trailing={summary ? steps : undefined}
      open={open}
      onToggle={() => setOpen((value) => !value)}
      // A wave holds a whole round of work — reasoning plus every tool call in
      // it — so it is routinely taller than the reading column. Its sticky header
      // keeps the count and round identity visible while the body scrolls.
      stickyHeader
    >
      {/* Each member already knows it is superseded — this wave exists BECAUSE an
          answer followed it — so nothing inside springs open when the wave does.
          Same seam owner as the transcript: a fold holds the rows it would otherwise
          have shown inline, so it has to space them the same way. */}
      {units.map((unit, index) => (
        <div key={index} className={unitSeamClass(units[index - 1], unit)}>
          {renderUnit(unit, facts, ctx)}
        </div>
      ))}
    </AgentActivityDisclosure>
  );
}
