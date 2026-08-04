import { createSingletonPort } from "@/lib/ports/singletonPort";

// The content-addressed handle an approve/reject decision acts on. revision binds
// the name to the exact reviewed bytes, so the runtime rejects a decision that
// would act on a proposal that changed under the reviewer; scope says which
// library an approval publishes into.
export interface SkillProposalHandle {
  name: string;
  revision: string;
  scope: "project" | "user";
}

// SkillProposalsGateway acts on the offline HITL review queue of skill proposals:
// approve one into the active library, or reject (discard) it. The runtime adapter
// drives skills.proposals.* over RPC against the bound workspace.
export interface SkillProposalsGateway {
  approve(handle: SkillProposalHandle): Promise<void>;
  reject(handle: SkillProposalHandle): Promise<void>;
}

const port = createSingletonPort<SkillProposalsGateway>(
  "Skill proposals gateway is not configured",
);

export const configureSkillProposalsGateway = port.configure;
export const skillProposalsGateway = port.get;
