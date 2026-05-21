// Gateway contract for the HITL permission flow.
//
// "Gateway" in the Clean-Architecture sense: a domain-defined interface
// for an outbound side-effect. The DOMAIN declares the call shape it
// needs; INFRA implements it against a concrete transport (HTTP today,
// could be Wails IPC / WebSocket / gRPC tomorrow). UI code never sees
// the transport — it asks the container for the gateway and calls
// submit().
//
// Errors throw — implementations should reject with a typed error so
// the caller can react (retry, show a toast, etc.). For now we don't
// model error types beyond "it threw"; introduce a `domain/errors/`
// hierarchy if richer recovery becomes useful.

import type { ApprovalSubmission } from "@/domain/models/Approval";

export interface PermissionGateway {
  submit(submission: ApprovalSubmission): Promise<void>;
}
