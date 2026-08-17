export interface SkillProposalHandle {
  workspace: string;
  name: string;
  revision: string;
  scope: "project" | "user";
}

export interface SkillCurationGateway {
  archive(name: string): Promise<void>;
  restore(name: string): Promise<void>;
  approveProposal(handle: SkillProposalHandle): Promise<void>;
  rejectProposal(handle: SkillProposalHandle): Promise<void>;
}
