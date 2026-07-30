import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveSessionTimeline,
  useActiveSessionToolCalls,
  useCurrentRootRunId,
  useIsCurrentRootRunning,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveSessionTimeline();
  const toolCalls = useActiveSessionToolCalls();
  const runId = useCurrentRootRunId();
  const running = useIsCurrentRootRunning();

  return useMemo(
    () => deriveLatestRun({ timeline, toolCalls, runId, running }),
    [timeline, toolCalls, runId, running],
  );
}
