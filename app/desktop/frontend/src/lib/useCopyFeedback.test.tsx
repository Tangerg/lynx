import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useCopyFeedback } from "./useCopyFeedback";

const copyText = vi.hoisted(() => vi.fn<(text: string) => Promise<boolean>>());

vi.mock("./clipboard", () => ({ copyText }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function CopyHarness({ material }: { material: string }) {
  const feedback = useCopyFeedback(material);
  return (
    <button type="button" aria-label={feedback.copied ? "copied" : "copy"} onClick={feedback.copy}>
      {material}
    </button>
  );
}

afterEach(() => {
  cleanup();
  copyText.mockReset();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("useCopyFeedback", () => {
  it("lets only the latest intent publish feedback for the same material", async () => {
    const older = deferred<boolean>();
    const latest = deferred<boolean>();
    copyText.mockReturnValueOnce(older.promise).mockReturnValueOnce(latest.promise);
    render(<CopyHarness material="same" />);

    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    await act(async () => latest.resolve(false));
    await act(async () => older.resolve(true));

    expect(screen.getByRole("button", { name: "copy" })).toBeTruthy();
  });

  it("retires the feedback timer with its mounted owner", async () => {
    vi.useFakeTimers();
    copyText.mockResolvedValueOnce(true);
    const view = render(<CopyHarness material="same" />);

    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    await act(async () => {});
    expect(screen.getByRole("button", { name: "copied" })).toBeTruthy();
    expect(vi.getTimerCount()).toBe(1);

    view.unmount();

    expect(vi.getTimerCount()).toBe(0);
  });
});
