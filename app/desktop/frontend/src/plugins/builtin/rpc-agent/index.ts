// Built-in plugin: drives the chat over the Lyra Runtime Protocol
// (JSON-RPC `runs.start` + `notifications/run/event`) instead of the
// legacy `@ag-ui/client` HttpAgent against POST /run.
//
// We only implement AbstractAgent's one abstract method — `run(input)` —
// and let the base class handle messages / subscribe / runAgent / abortRun
// exactly as it does for HttpAgent. So `useAgentSession` is unchanged; only
// the transport underneath swaps.
//
// Registered at priority 1 so it wins over the priority-0 http-agent, which
// stays registered as a fallback during the cutover.

import { AbstractAgent } from "@ag-ui/client";
import type { BaseEvent, RunAgentInput } from "@ag-ui/core";
import { Observable } from "rxjs";
import { definePlugin } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import type { Message } from "@/rpc";
import { asRunId, asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";

class RpcAgent extends AbstractAgent {
  run(input: RunAgentInput): Observable<BaseEvent> {
    return new Observable<BaseEvent>((observer) => {
      const controller = new AbortController();
      let runId: string | null = null;

      void (async () => {
        try {
          const { result, events } = await getContainer()
            .methods()
            .runs.start(
              {
                sessionId: asSessionId(input.threadId),
                runId: input.runId ? asRunId(input.runId) : undefined,
                // The Go runtime reads id/role/content; the protocol Message
                // shape carries more, but the cast is safe for the fields it
                // actually consumes (full message-shape alignment is tracked).
                messages: input.messages as unknown as Message[],
              },
              controller.signal,
            );
          runId = result.runId;
          for await (const event of events) observer.next(event);
          observer.complete();
        } catch (err) {
          observer.error(err);
        }
      })();

      // Unsubscribe = abortRun(): stop consuming AND tell the runtime to
      // stop the run server-side (best-effort).
      return () => {
        controller.abort();
        if (runId)
          void getContainer()
            .methods()
            .runs.cancel(asRunId(runId))
            .catch(() => undefined);
      };
    });
  }
}

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  version: "1.0.0",
  setup({ host }) {
    host.agent.registerSource({
      id: "rpc",
      label: "Runtime Protocol (JSON-RPC)",
      priority: 1,
      factory: () => {
        const active = useSessionStore.getState().activeSessionId;
        return new RpcAgent({ threadId: active || "s1" });
      },
    });
  },
});
