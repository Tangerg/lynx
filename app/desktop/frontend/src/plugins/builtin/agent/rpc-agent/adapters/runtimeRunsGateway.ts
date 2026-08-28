import { getContainer } from "@/main/container";
import { asItemId, asRunId, asSegmentId, asSessionId, type StartRunResponse } from "@/rpc";
import type {
  RpcRunResumeParams,
  RpcRunsGateway,
  RpcRunStartParams,
} from "../application/rpcAgentDriver";
import { createRunOpeningSettler } from "./runOpeningSettlement";

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
export interface RuntimeRunsGateway extends RpcRunsGateway {
  /** Retire every opening and stream admitted by the previous Runtime process. */
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

class DefaultRuntimeRunsGateway implements RuntimeRunsGateway {
  #openings = createRunOpeningSettler();

  async start({ sessionId, ...params }: RpcRunStartParams, signal?: AbortSignal) {
    const client = getContainer().client();
    const { result, events } = await this.#openings.settle(
      runOpeningIdentity("start", { sessionId, ...params }),
      (attemptSignal) =>
        client.runs.start({ ...params, sessionId: asSessionId(sessionId) }, attemptSignal),
      signal,
    );
    return { result: brandStartedRun(result), events };
  }

  async resume(params: RpcRunResumeParams, signal?: AbortSignal) {
    const client = getContainer().client();
    const { result, events } = await this.#openings.settle(
      runOpeningIdentity("resume", params),
      (attemptSignal) => client.runs.resume(params, attemptSignal),
      signal,
    );
    return {
      result: {
        runId: asRunId(result.runId),
        segmentId: asSegmentId(result.segmentId),
        ...(result.userItemId ? { userItemId: asItemId(result.userItemId) } : {}),
      },
      events,
    };
  }

  replaceRuntimeGeneration(): void {
    const predecessor = this.#openings;
    this.#openings = createRunOpeningSettler();
    predecessor.dispose();
  }

  dispose(): void {
    this.#openings.dispose();
  }
}

export function runtimeRunsGateway(): RuntimeRunsGateway {
  return new DefaultRuntimeRunsGateway();
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
