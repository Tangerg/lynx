// Domain layer barrel.
//
// Anything imported from `@/domain` should be pure types / interfaces /
// plain functions — zero React, zero fetch, zero zustand. The strict
// rule "domain depends on nothing" lets us swap infra / transports
// without ever touching this directory.
//
// Layout convention:
//   models/    — value types (e.g. ApprovalSubmission, ApprovalDecision)
//   gateways/  — outbound side-effect contracts (e.g. PermissionGateway)
//   errors/    — typed domain errors (when we need them; YAGNI until then)

export type { ApprovalDecision, ApprovalSubmission } from "./models/Approval";
export type { PermissionGateway } from "./gateways/PermissionGateway";
