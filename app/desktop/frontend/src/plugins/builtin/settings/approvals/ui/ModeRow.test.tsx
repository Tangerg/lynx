import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ApprovalMode } from "../application/approvalConfig";

const model = vi.hoisted(() => ({
  saveApprovalMode: vi.fn(),
}));

vi.mock("../application/approvalConfig", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/approvalConfig")>()),
  saveApprovalMode: model.saveApprovalMode,
  agentCommandWasRetired: () => false,
}));

import { ModeRow } from "./ModeRow";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  model.saveApprovalMode.mockReset();
});

describe("ModeRow", () => {
  it("owns the visible selection and duplicate admission while a save is pending", async () => {
    const saving = deferred<ApprovalMode>();
    model.saveApprovalMode.mockReturnValue(saving.promise);
    const view = render(<ModeRow mode="balanced" />);

    const safe = screen.getByRole("button", { name: "Safe" });
    const balanced = screen.getByRole("button", { name: "Balanced" });
    const auto = screen.getByRole("button", { name: "Auto" });
    fireEvent.click(auto);

    const pendingSelection = {
      auto: auto.getAttribute("aria-pressed"),
      balanced: balanced.getAttribute("aria-pressed"),
      busy: auto.getAttribute("aria-busy"),
      disabled: [safe, balanced, auto].every((button) => button.hasAttribute("disabled")),
    };
    fireEvent.click(safe);
    const callsWhileSaving = model.saveApprovalMode.mock.calls.length;

    await act(async () => {
      saving.resolve("yolo");
      await saving.promise;
    });
    const acceptedBeforeProjection = {
      auto: auto.getAttribute("aria-pressed"),
      balanced: balanced.getAttribute("aria-pressed"),
      disabled: [safe, balanced, auto].every((button) => button.hasAttribute("disabled")),
    };

    view.rerender(<ModeRow mode="yolo" />);
    await waitFor(() => expect(auto.hasAttribute("disabled")).toBe(false));

    expect(pendingSelection).toEqual({
      auto: "true",
      balanced: "false",
      busy: "true",
      disabled: true,
    });
    expect(callsWhileSaving).toBe(1);
    expect(acceptedBeforeProjection).toEqual({
      auto: "true",
      balanced: "false",
      disabled: true,
    });
    expect(auto.getAttribute("aria-pressed")).toBe("true");
  });

  it("retires a rejected intent and admits a corrected choice", async () => {
    const rejected = deferred<ApprovalMode>();
    model.saveApprovalMode.mockReturnValueOnce(rejected.promise).mockResolvedValueOnce("safe");
    render(<ModeRow mode="balanced" />);

    const safe = screen.getByRole("button", { name: "Safe" });
    const balanced = screen.getByRole("button", { name: "Balanced" });
    const auto = screen.getByRole("button", { name: "Auto" });
    fireEvent.click(auto);
    await act(async () => {
      rejected.reject(new Error("not saved"));
      await rejected.promise.catch(() => undefined);
    });

    await waitFor(() => expect(auto.hasAttribute("disabled")).toBe(false));
    expect(balanced.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(safe);

    expect(model.saveApprovalMode.mock.calls.map(([mode]) => mode)).toEqual(["yolo", "safe"]);
  });
});
