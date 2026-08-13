import { getContainer } from "@/main/container";
import { asItemId, asRunId, asSegmentId, asSessionId, type StartRunResponse } from "@/rpc";
import type { RpcRunsGateway } from "../application/rpcAgentDriver";
import { createRunOpeningSettler } from "./runOpeningSettlement";

const runOpeningSettler = createRunOpeningSettler();

function runOpeningIdentity(method: "start" | "resume", params: unknown): string {
  return JSON.stringify([`runs.${method}`, params]);
}

/**
 * The runs gateway the RPC agent source drives, bound to the live client.
 *
 * This lived as an object literal inside the plugin's `setup()`, which put the
 * two things a root is not supposed to hold — reaching the composition root, and
 * coercing the app's session id into the wire's branded one — in the file whose
 * whole job is assembly. Every sibling context keeps that pair in `adapters/`;
 * the rule was written down for `defaults/` only, so it read as a local
 * exception rather than the shape it is.
 */
export function runtimeRunsGateway(): RpcRunsGateway {
  return {
    start: async ({ sessionId, ...params }, signal) => {
      const client = getContainer().client();
      const { result, events } = await runOpeningSettler.settle(
        runOpeningIdentity("start", { sessionId, ...params }),
        (attemptSignal) =>
          client.runs.start({ ...params, sessionId: asSessionId(sessionId) }, attemptSignal),
        signal,
      );
      return { result: brandStartedRun(result), events };
    },
    resume: async (params, signal) => {
      const client = getContainer().client();
      const { result, events } = await runOpeningSettler.settle(
        runOpeningIdentity("resume", params),
        (attemptSignal) => client.runs.resume(params, attemptSignal),
        signal,
      );
      return {
        result: { runId: asRunId(result.runId), segmentId: asSegmentId(result.segmentId) },
        events,
      };
    },
  };
}

/**
 * The wire carries ids as plain strings — `ids.ts` brands them at the parse site,
 * and for a run's ids this adapter IS that site. The app's ports speak branded ids
 * so a RunId can never be passed where an ItemId belongs.
 */
function brandStartedRun(result: StartRunResponse) {
  return {
    runId: asRunId(result.runId),
    // The segment, not just the run: a stream is a segment's, and reattaching after a
    // dropped connection has to name the one it was following.
    segmentId: asSegmentId(result.segmentId),
    userItemId: asItemId(result.userItemId),
  };
}
