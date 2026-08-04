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

import { useState } from "react";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import { waveStepCount } from "@/plugins/builtin/agent/public/messagePresentation";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type { BlockCtx } from "./blockContext";
import { renderUnit } from "./BlockRenderer";

export function NarrativeWave({ units, ctx }: { units: MessageRenderUnit[]; ctx: BlockCtx }) {
  const t = useT();
  const [open, setOpen] = useState(false);

  return (
    <AgentActivityDisclosure
      icon="history"
      shell="line"
      // The count IS the label. A word for what this is would have to be a tense, and
      // the only tense available is past — the wave exists because an answer followed
      // it — while the glyph beside it already says "what happened".
      label={t("narrative.wave.steps", { count: waveStepCount(units) })}
      open={open}
      onToggle={() => setOpen((value) => !value)}
    >
      {/* Each member already knows it is superseded — this wave exists BECAUSE an
          answer followed it — so nothing inside springs open when the wave does. */}
      {units.map((unit, index) => (
        <div key={index}>{renderUnit(unit, ctx)}</div>
      ))}
    </AgentActivityDisclosure>
  );
}
