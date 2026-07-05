type ScheduleFrame = (callback: () => void) => void;

interface RecallHistoryOptions {
  event: KeyboardEvent;
  recall: () => boolean;
  scheduleFrame?: ScheduleFrame;
}

function targetTextarea(event: KeyboardEvent): HTMLTextAreaElement | null {
  return event.target instanceof HTMLTextAreaElement ? event.target : null;
}

function caretToEnd(textarea: HTMLTextAreaElement, scheduleFrame: ScheduleFrame): void {
  scheduleFrame(() => {
    const end = textarea.value.length;
    textarea.setSelectionRange(end, end);
  });
}

function collapsedCaret(textarea: HTMLTextAreaElement): boolean {
  return textarea.selectionStart === textarea.selectionEnd;
}

export function recallPreviousHistoryFromKey({
  event,
  recall,
  scheduleFrame = requestAnimationFrame,
}: RecallHistoryOptions): boolean {
  const textarea = targetTextarea(event);
  if (!textarea || !collapsedCaret(textarea)) return false;
  if (textarea.value.slice(0, textarea.selectionStart).includes("\n")) return false;
  if (!recall()) return false;
  caretToEnd(textarea, scheduleFrame);
  return true;
}

export function recallNextHistoryFromKey({
  event,
  recall,
  scheduleFrame = requestAnimationFrame,
}: RecallHistoryOptions): boolean {
  const textarea = targetTextarea(event);
  if (!textarea || !collapsedCaret(textarea)) return false;
  if (textarea.value.slice(textarea.selectionEnd).includes("\n")) return false;
  if (!recall()) return false;
  caretToEnd(textarea, scheduleFrame);
  return true;
}
