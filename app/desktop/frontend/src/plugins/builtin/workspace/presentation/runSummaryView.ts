import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveSessionTimeline,
  useActiveSessionToolCalls,
  useCurrentRootAttention,
  useCurrentRootOutcome,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveSessionTimeline();
  const toolCalls = useActiveSessionToolCalls();
  const attention = useCurrentRootAttention();
  const outcome = useCurrentRootOutcome();

  return useMemo(
    () => deriveLatestRun({ timeline, toolCalls, attention, outcome }),
    [timeline, toolCalls, attention, outcome],
  );
}
