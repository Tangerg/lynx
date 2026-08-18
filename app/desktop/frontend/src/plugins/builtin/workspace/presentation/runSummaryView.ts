import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveSessionTimeline,
  useActiveSessionToolCalls,
  useCurrentRootMaterial,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveSessionTimeline();
  const toolCalls = useActiveSessionToolCalls();
  const root = useCurrentRootMaterial();

  return useMemo(
    () =>
      deriveLatestRun({
        timeline,
        toolCalls,
        attention: root.attention,
        outcome: root.outcome,
      }),
    [timeline, toolCalls, root],
  );
}
