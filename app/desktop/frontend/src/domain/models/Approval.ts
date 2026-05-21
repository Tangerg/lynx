// Domain model for the HITL approval flow.
//
// Lives in `domain/` so the rest of the code (UI hooks, infra gateways,
// plugin handlers) imports from a single source of truth that knows
// nothing about transports (HTTP, WebSocket, IPC) or storage. If we
// ever swap the backend protocol the gateway implementation changes
// but this file doesn't.

export type ApprovalDecision = "approved" | "declined";

export type ApprovalSubmission = {
  requestId: string;
  decision: ApprovalDecision;
};
