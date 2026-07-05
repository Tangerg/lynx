import { describe, expect, it, vi } from "vitest";
import { recallNextHistoryFromKey, recallPreviousHistoryFromKey } from "./composerHistoryKeys";

function keyEvent(target: EventTarget): KeyboardEvent {
  const event = new KeyboardEvent("keydown");
  Object.defineProperty(event, "target", { value: target });
  return event;
}

function textarea(value: string, selectionStart: number, selectionEnd = selectionStart) {
  const el = document.createElement("textarea");
  el.value = value;
  el.setSelectionRange(selectionStart, selectionEnd);
  return el;
}

const runFrame = (callback: () => void) => callback();

describe("composer history key handling", () => {
  it("recalls previous history only from the first line", () => {
    const el = textarea("first\nsecond", 3);
    const recall = vi.fn(() => true);

    expect(
      recallPreviousHistoryFromKey({ event: keyEvent(el), recall, scheduleFrame: runFrame }),
    ).toBe(true);

    expect(recall).toHaveBeenCalledOnce();
    expect(el.selectionStart).toBe(el.value.length);
    expect(el.selectionEnd).toBe(el.value.length);
  });

  it("does not recall previous history from later lines or selected text", () => {
    const recall = vi.fn(() => true);

    expect(
      recallPreviousHistoryFromKey({
        event: keyEvent(textarea("first\nsecond", 8)),
        recall,
        scheduleFrame: runFrame,
      }),
    ).toBe(false);
    expect(
      recallPreviousHistoryFromKey({
        event: keyEvent(textarea("first", 1, 3)),
        recall,
        scheduleFrame: runFrame,
      }),
    ).toBe(false);

    expect(recall).not.toHaveBeenCalled();
  });

  it("recalls next history only from the last line", () => {
    const el = textarea("first\nsecond", 10);
    const recall = vi.fn(() => true);

    expect(recallNextHistoryFromKey({ event: keyEvent(el), recall, scheduleFrame: runFrame })).toBe(
      true,
    );

    expect(recall).toHaveBeenCalledOnce();
    expect(el.selectionStart).toBe(el.value.length);
    expect(el.selectionEnd).toBe(el.value.length);
  });

  it("does not recall next history from earlier lines or selected text", () => {
    const recall = vi.fn(() => true);

    expect(
      recallNextHistoryFromKey({
        event: keyEvent(textarea("first\nsecond", 3)),
        recall,
        scheduleFrame: runFrame,
      }),
    ).toBe(false);
    expect(
      recallNextHistoryFromKey({
        event: keyEvent(textarea("second", 1, 4)),
        recall,
        scheduleFrame: runFrame,
      }),
    ).toBe(false);

    expect(recall).not.toHaveBeenCalled();
  });

  it("leaves the caret alone when the history port has no entry", () => {
    const el = textarea("first", 2);

    expect(
      recallPreviousHistoryFromKey({
        event: keyEvent(el),
        recall: () => false,
        scheduleFrame: runFrame,
      }),
    ).toBe(false);

    expect(el.selectionStart).toBe(2);
    expect(el.selectionEnd).toBe(2);
  });
});
