import type { Item, RunEvent } from "@/rpc";

export interface AgentFoldSource {
  runId: string;
  segmentId: string | null;
  eventId: string;
  timestamp: string;
}

export function runEventSource(event: RunEvent): AgentFoldSource {
  return {
    runId: event.runId,
    segmentId: event.segmentId,
    eventId: event.eventId,
    timestamp: event.timestamp,
  };
}

export function durableItemSource(item: Item): AgentFoldSource {
  return {
    runId: item.runId,
    segmentId: null,
    eventId: `history:${item.id}:completed`,
    timestamp: item.createdAt,
  };
}

export function sourceTimestamp(source: AgentFoldSource): number {
  const timestamp = Date.parse(source.timestamp);
  if (Number.isNaN(timestamp)) {
    throw new Error(
      `agent.fold.timestampInvalid:event=${source.eventId};run=${source.runId};timestamp=${source.timestamp}`,
    );
  }
  return timestamp;
}
