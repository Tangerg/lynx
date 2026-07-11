import type { ContentBlock, RunEvent } from "@/rpc";
import { LOCAL_MESSAGE_PREFIX } from "@/plugins/sdk/types/agentView";

let localSeq = 0;

export interface OptimisticUserMessage {
  localId: string;
  event: RunEvent["event"];
}

export function createOptimisticUserMessage(content: ContentBlock[]): OptimisticUserMessage {
  const localId = `${LOCAL_MESSAGE_PREFIX}${++localSeq}`;
  return {
    localId,
    event: {
      type: "item.completed",
      item: {
        id: localId,
        runId: "",
        status: "completed",
        createdAt: new Date().toISOString(),
        type: "userMessage",
        content,
      },
    } as RunEvent["event"],
  };
}
