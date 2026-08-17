import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import type { PendingInterruptGroup } from "@/plugins/sdk/types/agentSessionView";
import {
  discardStagedInterruptResponses,
  installInterruptResponseCoordinator,
  interruptResponseIsStaged,
  stageInterruptResponse,
} from "./interruptResponseCoordinator";

const SESSION_ID = "ses_barrier";
const ROOT_RUN_ID = "run_root";
let disposeCoordinator: () => void = () => undefined;

function seedPending(groups: PendingInterruptGroup[]): void {
  const store = useAgentStore.getState();
  store.ensureSession(SESSION_ID);
  const token = store.beginViewRefresh(SESSION_ID, false)!;
  const view = useAgentStore.getState().sessions[SESSION_ID]!.view;
  store.commitViewRefresh(SESSION_ID, token, { ...view, pendingInterrupts: groups });
}

function groups(): PendingInterruptGroup[] {
  return [
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
      interrupts: [{ itemId: "question_b", kind: "question" }],
    },
  ];
}

beforeEach(() => {
  discardStagedInterruptResponses();
  useAgentStore.getState().dropSession(SESSION_ID);
  disposeCoordinator = installInterruptResponseCoordinator();
});

afterEach(() => {
  disposeCoordinator();
  discardStagedInterruptResponses();
  useAgentStore.getState().dropSession(SESSION_ID);
});

describe("interrupt response coordinator", () => {
  it("collects every child response and resumes the owning root once", () => {
    seedPending(groups());
    let accept: (() => void) | undefined;
    const resume = vi.fn((_runId, _responses, onSettled?: () => void) => {
      accept = onSettled;
      return true;
    });
    useAgentStore.getState().setResume(SESSION_ID, resume);
    const approvalSettled = vi.fn();
    const questionSettled = vi.fn();

    expect(
      stageInterruptResponse(
        SESSION_ID,
        ROOT_RUN_ID,
        "approval_a",
        { type: "approval", decision: "approve" },
        { decision: "approved" },
        { onSettled: approvalSettled },
      ),
    ).toBe(true);
    expect(resume).not.toHaveBeenCalled();
    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(true);

    expect(
      stageInterruptResponse(
        SESSION_ID,
        ROOT_RUN_ID,
        "question_b",
        { type: "answer", answers: [["Postgres"]] },
        { answered: true, answers: [["Postgres"]] },
        { onSettled: questionSettled },
      ),
    ).toBe(true);

    expect(resume).toHaveBeenCalledOnce();
    expect(resume).toHaveBeenCalledWith(
      ROOT_RUN_ID,
      [
        {
          itemId: "approval_a",
          response: { type: "approval", decision: "approve" },
        },
        {
          itemId: "question_b",
          response: { type: "answer", answers: [["Postgres"]] },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
    expect(approvalSettled).not.toHaveBeenCalled();
    expect(questionSettled).not.toHaveBeenCalled();

    accept?.();
    expect(approvalSettled).toHaveBeenCalledOnce();
    expect(questionSettled).toHaveBeenCalledOnce();
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.pendingInterrupts).toEqual([]);
  });

  it("rolls the whole staged barrier back when run opening is rejected", () => {
    seedPending(groups());
    const approvalError = vi.fn();
    const questionError = vi.fn();
    const resume = vi.fn((_runId, _responses, _onSettled, onError?: () => void) => {
      onError?.();
      return true;
    });
    useAgentStore.getState().setResume(SESSION_ID, resume);

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "deny" },
      { decision: "declined" },
      { onError: approvalError },
    );
    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "question_b",
      { type: "answer", answers: [["SQLite"]] },
      { answered: true, answers: [["SQLite"]] },
      { onError: questionError },
    );

    expect(approvalError).toHaveBeenCalledOnce();
    expect(questionError).toHaveBeenCalledOnce();
    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(false);
    expect(useAgentStore.getState().sessions[SESSION_ID]!.view.pendingInterrupts).toEqual(groups());
  });

  it("releases every card when another local command owns run opening", () => {
    seedPending(groups());
    const approvalError = vi.fn();
    const questionError = vi.fn();
    useAgentStore.getState().setResume(
      SESSION_ID,
      vi.fn(() => false),
    );

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "approve" },
      { decision: "approved" },
      { onError: approvalError },
    );
    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "question_b",
      { type: "answer", answers: [["Postgres"]] },
      { answered: true, answers: [["Postgres"]] },
      { onError: questionError },
    );

    expect(approvalError).toHaveBeenCalledOnce();
    expect(questionError).toHaveBeenCalledOnce();
    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(false);
  });

  it("discards a partial barrier when an authoritative refresh removes its set", () => {
    seedPending(groups());
    useAgentStore.getState().setResume(
      SESSION_ID,
      vi.fn(() => true),
    );
    const onError = vi.fn();
    const dispose = installInterruptResponseCoordinator();

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "approve" },
      { decision: "approved" },
      { onError },
    );
    seedPending([]);

    expect(onError).toHaveBeenCalledOnce();
    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(false);
    dispose();
  });

  it("retires a submitting batch when authoritative projection shows a remote continuation", () => {
    seedPending(groups());
    let accept: (() => void) | undefined;
    let reject: (() => boolean | void) | undefined;
    const resume = vi.fn((_runId, _responses, onSettled, onStartError) => {
      accept = onSettled;
      reject = onStartError;
      return true;
    });
    useAgentStore.getState().setResume(SESSION_ID, resume);
    const approvalSettled = vi.fn();
    const questionSettled = vi.fn();
    const approvalError = vi.fn();
    const questionError = vi.fn();
    const dispose = installInterruptResponseCoordinator();

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "approve" },
      { decision: "approved" },
      { onSettled: approvalSettled, onError: approvalError },
    );
    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "question_b",
      { type: "answer", answers: [["Postgres"]] },
      { answered: true, answers: [["Postgres"]] },
      { onSettled: questionSettled, onError: questionError },
    );

    // Another client consumed the atomic set while this client's resume ack
    // remained in flight. The durable projection is now the only answer.
    seedPending([]);

    expect(approvalError).toHaveBeenCalledOnce();
    expect(questionError).toHaveBeenCalledOnce();
    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(false);
    expect(reject?.()).toBe(true);

    // A late local ack must not paint this client's choices over the already
    // materialized authoritative result.
    accept?.();
    expect(approvalSettled).not.toHaveBeenCalled();
    expect(questionSettled).not.toHaveBeenCalled();
    dispose();
  });

  it("does not let a retired reconciliation clear its successor's staged barrier", () => {
    seedPending(groups());
    useAgentStore.getState().setResume(
      SESSION_ID,
      vi.fn(() => true),
    );
    const retired = installInterruptResponseCoordinator();
    const successor = installInterruptResponseCoordinator();
    const onError = vi.fn();

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "approve" },
      { decision: "approved" },
      { onError },
    );
    retired();

    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(true);
    expect(onError).not.toHaveBeenCalled();
    successor();
  });

  it("retires the old submitted barrier before its successor accepts the same identity", () => {
    seedPending(groups());
    let retiredAccept: (() => void) | undefined;
    const retiredResume = vi.fn((_runId, _responses, onSettled) => {
      retiredAccept = onSettled;
      return true;
    });
    useAgentStore.getState().setResume(SESSION_ID, retiredResume);
    const retiredApprovalSettled = vi.fn();
    const retiredQuestionSettled = vi.fn();
    const retiredApprovalError = vi.fn();
    const retiredQuestionError = vi.fn();
    const retired = installInterruptResponseCoordinator();

    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "approval_a",
      { type: "approval", decision: "approve" },
      { decision: "approved" },
      { onSettled: retiredApprovalSettled, onError: retiredApprovalError },
    );
    stageInterruptResponse(
      SESSION_ID,
      ROOT_RUN_ID,
      "question_b",
      { type: "answer", answers: [["Retired"]] },
      { answered: true, answers: [["Retired"]] },
      { onSettled: retiredQuestionSettled, onError: retiredQuestionError },
    );
    expect(retiredResume).toHaveBeenCalledOnce();

    const successor = installInterruptResponseCoordinator();
    const successorResume = vi.fn(() => true);
    useAgentStore.getState().setResume(SESSION_ID, successorResume);
    const successorError = vi.fn();

    expect(retiredApprovalError).toHaveBeenCalledOnce();
    expect(retiredQuestionError).toHaveBeenCalledOnce();
    expect(
      stageInterruptResponse(
        SESSION_ID,
        ROOT_RUN_ID,
        "approval_a",
        { type: "approval", decision: "deny" },
        { decision: "declined" },
        { onError: successorError },
      ),
    ).toBe(true);

    retiredAccept?.();

    expect(interruptResponseIsStaged(SESSION_ID, ROOT_RUN_ID, "approval_a")).toBe(true);
    expect(successorError).not.toHaveBeenCalled();
    expect(retiredApprovalSettled).not.toHaveBeenCalled();
    expect(retiredQuestionSettled).not.toHaveBeenCalled();
    retired();
    successor();
  });
});
