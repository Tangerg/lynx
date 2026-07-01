import { useMemo } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  useActiveRunTimeline,
  useActiveRunToolCalls,
  useIsAgentRunning,
} from "@/plugins/builtin/agent/public/run";
import { deriveLatestRun } from "@/plugins/builtin/agent/public/runDigest";
import { INITIAL_VIEW_STATE } from "@/plugins/builtin/agent/public/viewState";

export function useLatestRunDigest(): RunDigest | null {
  const timeline = useActiveRunTimeline();
  const toolCalls = useActiveRunToolCalls();
  const running = useIsAgentRunning();

  return useMemo(
    () =>
      deriveLatestRun({
        ...INITIAL_VIEW_STATE,
        timeline,
        toolCalls,
        run: { ...INITIAL_VIEW_STATE.run, running },
      }),
    [timeline, toolCalls, running],
  );
}
