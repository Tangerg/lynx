import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToolOutputPanel } from "./ToolOutputPanel";

const copyText = vi.hoisted(() => vi.fn<(text: string) => Promise<boolean>>());

vi.mock("@/lib/clipboard", () => ({ copyText }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  copyText.mockReset();
});

describe("ToolOutputPanel copy material ownership", () => {
  it("does not lend a retired streaming-output copy response to the replacement output", async () => {
    const retiredCopy = deferred<boolean>();
    copyText.mockReturnValueOnce(retiredCopy.promise);
    const view = render(<ToolOutputPanel output="old output" status="running" />);

    fireEvent.click(screen.getByRole("button", { name: "Copy output" }));
    expect(copyText).toHaveBeenCalledWith("old output");

    view.rerender(<ToolOutputPanel output="replacement output" status="running" />);
    await act(async () => retiredCopy.resolve(true));

    expect(screen.getByRole("button", { name: "Copy output" })).toBeTruthy();
  });
});
