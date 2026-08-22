import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { navigator } from "@/lib/navigation";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import type { PendingInterruptGroup } from "@/plugins/sdk/types/agentSessionView";
import { installInterruptResponseCoordinator } from "./interruptResponseCoordinator";
import { registerApprovalActions } from "./useApprovalSubmit";
import { submitPendingApproval } from "./submitPendingApproval";

const SESSION_ID = "ses_keyboard_barrier";
const ROOT_RUN_ID = "run_root";
let disposeCoordinator: () => void = () => undefined;

function seedPending(groups: PendingInterruptGroup[]): void {
  const store = useAgentStore.getState();
  store.ensureSession(SESSION_ID);
  const token = store.beginViewRefresh(SESSION_ID, false)!;
  const view = useAgentStore.getState().sessions[SESSION_ID]!.view;
  store.commitViewRefresh(SESSION_ID, token, { ...view, pendingInterrupts: groups });
}

beforeEach(() => {
  useAgentStore.getState().dropSession(SESSION_ID);
  navigator().go({ session: SESSION_ID });
  disposeCoordinator = installInterruptResponseCoordinator();
});

afterEach(() => {
  disposeCoordinator();
  useAgentStore.getState().dropSession(SESSION_ID);
});

describe("submitPendingApproval", () => {
  it("walks an atomic child-run barrier and resumes its root once", () => {
    seedPending([
      {
        sessionId: SESSION_ID,
        runId: "run_child_a",
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_a", kind: "approval" }],
      },
      {
        sessionId: SESSION_ID,
        runId: "run_child_b",
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_b", kind: "approval" }],
      },
    ]);
    let accept: (() => void) | undefined;
    const resume = vi.fn((_runId, _responses, onSettled?: () => void) => {
      accept = onSettled;
      return true;
    });
    useAgentStore.getState().setResume(SESSION_ID, resume);

    expect(submitPendingApproval("approved")).toBe(true);
    expect(resume).not.toHaveBeenCalled();
    expect(submitPendingApproval("declined")).toBe(true);
    expect(resume).toHaveBeenCalledWith(
      ROOT_RUN_ID,
      [
        {
          itemId: "approval_a",
          response: { type: "approval", decision: "approve" },
        },
        {
          itemId: "approval_b",
          response: { type: "approval", decision: "deny" },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
    // While the barrier is opening, the shortcut is consumed and cannot fall
    // through to sending composer text as a new Run.
    expect(submitPendingApproval("approved")).toBe(true);

    accept?.();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.pendingInterrupts).toEqual([]);
  });

  it("does not consume the approval shortcut for a question-only barrier", () => {
    seedPending([
      {
        sessionId: SESSION_ID,
        runId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "question_a", kind: "question" }],
      },
    ]);
    useAgentStore.getState().setResume(
      SESSION_ID,
      vi.fn(() => true),
    );

    expect(submitPendingApproval("approved")).toBe(false);
  });

  it("does not borrow a mounted approval card from another Session", () => {
    seedPending([
      {
        sessionId: SESSION_ID,
        runId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_same", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    useAgentStore.getState().setResume(SESSION_ID, resume);
    const retiredApprove = vi.fn();
    const unregister = registerApprovalActions("ses_retired", ROOT_RUN_ID, "approval_same", {
      approve: retiredApprove,
      decline: vi.fn(),
    });

    try {
      expect(submitPendingApproval("approved")).toBe(true);
      expect(retiredApprove).not.toHaveBeenCalled();
      expect(resume).toHaveBeenCalledOnce();
    } finally {
      unregister();
    }
  });

  it("does not borrow a mounted approval card from another root Run", () => {
    seedPending([
      {
        sessionId: SESSION_ID,
        runId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_same", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    useAgentStore.getState().setResume(SESSION_ID, resume);
    const retiredApprove = vi.fn();
    const unregister = registerApprovalActions(SESSION_ID, "run_retired", "approval_same", {
      approve: retiredApprove,
      decline: vi.fn(),
    });

    try {
      expect(submitPendingApproval("approved")).toBe(true);
      expect(retiredApprove).not.toHaveBeenCalled();
      expect(resume).toHaveBeenCalledOnce();
    } finally {
      unregister();
    }
  });

  it("uses the mounted card only for the exact approval owner", () => {
    seedPending([
      {
        sessionId: SESSION_ID,
        runId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "approval_exact", kind: "approval" }],
      },
    ]);
    const resume = vi.fn(() => true);
    useAgentStore.getState().setResume(SESSION_ID, resume);
    const approve = vi.fn();
    const unregister = registerApprovalActions(SESSION_ID, ROOT_RUN_ID, "approval_exact", {
      approve,
      decline: vi.fn(),
    });

    try {
      expect(submitPendingApproval("approved")).toBe(true);
      expect(approve).toHaveBeenCalledOnce();
      expect(resume).not.toHaveBeenCalled();
    } finally {
      unregister();
    }
  });
});
