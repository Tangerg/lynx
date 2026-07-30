import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveRunId,
  useActiveRunTimeline,
  useActiveRunToolCalls,
  useIsAgentRunning,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveRunTimeline();
  const toolCalls = useActiveRunToolCalls();
  const runId = useActiveRunId();
  const running = useIsAgentRunning();

  return useMemo(
    () => deriveLatestRun({ timeline, toolCalls, runId, running }),
    [timeline, toolCalls, runId, running],
  );
}
