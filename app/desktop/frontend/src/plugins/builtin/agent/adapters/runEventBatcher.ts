import type { RunEvent } from "@/rpc";

type ScheduleFrame = (flush: () => void) => number;
type CancelFrame = (handle: number) => void;

export interface RunEventBatcher {
  enqueue(event: RunEvent): void;
  dispose(): void;
}

interface RunEventBatcherOptions {
  readEpoch: () => number;
  apply: (batch: RunEvent[]) => void;
  onRunFinished?: () => void;
  scheduleFrame?: ScheduleFrame;
  cancelFrame?: CancelFrame;
}

export function createRunEventBatcher({
  readEpoch,
  apply,
  onRunFinished,
  scheduleFrame = requestAnimationFrame,
  cancelFrame = cancelAnimationFrame,
}: RunEventBatcherOptions): RunEventBatcher {
  let queue: RunEvent[] = [];
  let frame: number | null = null;
  let queueEpoch = readEpoch();
  let disposed = false;

  const flush = (): void => {
    frame = null;
    if (disposed || queue.length === 0) return;

    const batch = queue;
    queue = [];
    if (readEpoch() !== queueEpoch) {
      queueEpoch = readEpoch();
      return;
    }

    apply(batch);
    if (batch.some((entry) => entry.event.type === "segment.finished")) onRunFinished?.();
  };

  return {
    enqueue(event) {
      if (disposed) return;

      const epoch = readEpoch();
      if (epoch !== queueEpoch) {
        queue = [];
        queueEpoch = epoch;
      }
      queue.push(event);
      if (frame === null) frame = scheduleFrame(flush);
    },
    dispose() {
      disposed = true;
      queue = [];
      if (frame !== null) cancelFrame(frame);
      frame = null;
    },
  };
}
