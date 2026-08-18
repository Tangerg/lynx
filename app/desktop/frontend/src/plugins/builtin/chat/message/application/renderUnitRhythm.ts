import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";

/**
 * Which of the transcript's three voices a unit speaks in.
 *
 * `process` is what the turn DID (a thought, a call, a folded wave) — quiet rows
 * that belong to each other. `prose` is the answer. `panel` is something that
 * stands on its own and usually wants something from the reader (an approval, a
 * question, a plan, a compaction note, an image).
 */
export type UnitVoice = "process" | "prose" | "panel";

export function unitVoice(unit: MessageRenderUnit): UnitVoice {
  if (unit.kind === "wave" || unit.kind === "toolGroup") return "process";
  switch (unit.block.kind) {
    case "text":
      return "prose";
    case "tool":
    case "reasoning":
      return "process";
    default:
      return "panel";
  }
}

/**
 * How far apart two adjacent units sit — keyed on the PAIR, because a seam is a
 * relationship and neither side of it can know the distance alone.
 *
 * This is the fix for a real defect rather than a taste preference. Every card used
 * to carry its own outer margin: the activity disclosure `my-1`, reasoning and
 * delegated runs `my-1.5`, the HITL and question cards `my-2`, the plan and
 * compaction blocks `my-3`. Eight authors, eight answers, and adjacent margins
 * collapse — so the distance between any two units depended on which pair happened
 * to meet, with no rule anywhere. That is not one flat rhythm, it is noise, and
 * noise reads as flat.
 *
 * Three distances, and the ratio is the point: consecutive process rows stay close
 * (they are one act), a change of voice opens up, and a panel keeps its own air.
 * Cards no longer set outer margins at all — this table is the only owner.
 *
 * The absolute values are both references' measured answer: 20px where the voice
 * changes (JetBrains sets 20 between an assistant turn's blocks, Nova 18–26), and
 * 6px between adjacent activity rows (JetBrains 7 between tool rows). This keeps
 * work distinct from the answer while grouping adjacent activity rows.
 */
const SEAM: Record<UnitVoice, Record<UnitVoice, string>> = {
  process: { process: "mt-1.5", prose: "mt-5", panel: "mt-4" },
  prose: { process: "mt-5", prose: "mt-3", panel: "mt-4" },
  panel: { process: "mt-4", prose: "mt-5", panel: "mt-3" },
};

export function unitSeamClass(
  previous: MessageRenderUnit | undefined,
  unit: MessageRenderUnit,
): string {
  if (!previous) return "";
  return SEAM[unitVoice(previous)][unitVoice(unit)];
}

/**
 * How far in from the reading measure a unit starts.
 *
 * The other half of the hierarchy, and the half that works horizontally: the answer
 * owns the full measure, and the account of how it was reached is an aside set in
 * from it. Vertical distance alone cannot express that subordination.
 *
 * One step, and only where prose is a sibling. Inside a folded wave every member is
 * process, so indenting them all shifts the group without saying anything about it —
 * and the disclosure body already provides that group's own inset.
 */
// The top level starts where the sentence does. A step in from the measure was meant
// to say "this is subordinate", and at the top level there is nothing above it to be
// subordinate to — it just left the column.
const INDENT: Record<UnitVoice, string> = { process: "", prose: "", panel: "" };

export function unitIndentClass(unit: MessageRenderUnit): string {
  return INDENT[unitVoice(unit)];
}
