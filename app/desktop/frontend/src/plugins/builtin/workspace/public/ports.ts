// The Workspace context's setup-time contract — see `agent/public/ports` for why
// only setup-time readers need one.

import { service } from "dougong";

export interface WorkspaceScopePorts {
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

export const WORKSPACE_SCOPE_PORTS = service<WorkspaceScopePorts>("scopeapp.workspace.scopePorts");

export interface WorkspaceMutationLifecyclePorts {
  replaceRuntimeGeneration(): void;
}

export const WORKSPACE_MUTATION_LIFECYCLE_PORTS = service<WorkspaceMutationLifecyclePorts>(
  "scopeapp.workspace.mutationLifecyclePorts",
);
