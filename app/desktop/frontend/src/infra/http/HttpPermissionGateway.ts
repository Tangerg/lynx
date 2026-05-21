// HTTP implementation of the PermissionGateway domain contract.
//
// The only place in the app that knows the wire format of the
// /permission endpoint (POST application/json {requestId, decision}).
// If we ever move to a different transport — Wails IPC, WebSocket
// frames inside the AG-UI stream, gRPC — drop in a sibling
// IpcPermissionGateway / WsPermissionGateway, wire it in the
// container, UI doesn't change.
//
// Lives in `infra/` (sibling to the domain) so the dependency rule
// is plain: infra → domain, never the reverse.

import type { ApprovalSubmission, PermissionGateway } from "@/domain";

export class HttpPermissionGateway implements PermissionGateway {
  constructor(private readonly baseUrl: string) {}

  async submit(submission: ApprovalSubmission): Promise<void> {
    const response = await fetch(`${this.baseUrl}/permission`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(submission),
    });
    if (!response.ok) {
      const body = await response.text().catch(() => "");
      throw new Error(
        `permission submit ${response.status}${body ? `: ${body}` : ""}`,
      );
    }
  }
}
