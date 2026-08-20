import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  approvalSubmitOptions,
  canRegisterApprovalActions,
  useApprovalCardActions,
} from "./approvalCardActions";

type RegisteredApprovalActions = {
  approve: () => void;
  decline: () => void;
};

const hitl = vi.hoisted(() => ({
  registerActions: vi.fn<(actions: RegisteredApprovalActions) => () => void>(() => () => undefined),
  submit: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/hitl", () => ({
  useApprovalSubmit: () => ({
    pending: null,
    registerActions: hitl.registerActions,
    submit: hitl.submit,
  }),
}));

vi.mock("@/plugins/builtin/agent/public/messagePresentation", () => ({
  canSubmitApproval: () => true,
}));

afterEach(() => vi.clearAllMocks());

describe("approvalSubmitOptions", () => {
  it("omits the options object when approval has no extra payload", () => {
    expect(approvalSubmitOptions({})).toBeUndefined();
  });

  it("preserves edited args and remember scope", () => {
    expect(approvalSubmitOptions({ editedArgs: {}, rememberScope: "project" })).toEqual({
      editedArgs: {},
      rememberScope: "project",
    });
  });
});

describe("canRegisterApprovalActions", () => {
  it("registers shortcuts only for an open resumable approval", () => {
    expect(
      canRegisterApprovalActions({
        runId: "run",
        itemId: "item",
        status: "requires-action",
      }),
    ).toBe(true);
    expect(
      canRegisterApprovalActions({
        runId: "run",
        itemId: "item",
        status: "complete",
      }),
    ).toBe(false);
    expect(
      canRegisterApprovalActions({
        runId: undefined,
        itemId: "item",
        status: "requires-action",
      }),
    ).toBe(false);
    expect(
      canRegisterApprovalActions({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        runtimeAvailable: false,
      }),
    ).toBe(false);
  });
});

describe("useApprovalCardActions", () => {
  it("attaches a remembered scope only to the explicitly scoped approval", () => {
    const argsEditor = { commit: vi.fn(() => ({ path: "/safe" })) };
    const { result } = renderHook(() =>
      useApprovalCardActions({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        argsEditor,
        runtimeAvailable: true,
      }),
    );

    act(() => result.current.approve("project"));
    expect(hitl.submit).toHaveBeenLastCalledWith("approved", {
      editedArgs: { path: "/safe" },
      rememberScope: "project",
    });

    act(() => result.current.decline());
    expect(hitl.submit).toHaveBeenLastCalledWith("declined");
  });

  it("keeps the registered keyboard approval one-shot", () => {
    renderHook(() =>
      useApprovalCardActions({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        runtimeAvailable: true,
      }),
    );

    const registered = hitl.registerActions.mock.calls.at(-1)?.[0];
    act(() => registered?.approve());
    expect(hitl.submit).toHaveBeenLastCalledWith("approved", undefined);
  });
});
