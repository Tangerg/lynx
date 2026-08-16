import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveSessionTimeline,
  useActiveSessionToolCalls,
  useCurrentRootOutcome,
  useCurrentRootRunId,
  useIsCurrentRootRunning,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveSessionTimeline();
  const toolCalls = useActiveSessionToolCalls();
  const runId = useCurrentRootRunId();
  const running = useIsCurrentRootRunning();
  const outcome = useCurrentRootOutcome();

  return useMemo(
    () => deriveLatestRun({ timeline, toolCalls, runId, running, outcome }),
    [timeline, toolCalls, runId, running, outcome],
  );
}
