import { createSingletonPort } from "@/lib/ports/singletonPort";

// The content-addressed handle a promote/reject decision acts on. revision binds
// the name to the exact reviewed bytes, so the runtime rejects a decision that
// would act on a draft that changed under the reviewer.
export interface SkillDraftHandle {
  name: string;
  revision: string;
}

// SkillDraftsGateway acts on the offline HITL review queue of agent-mined skill
// proposals: promote one into the active library, or reject (discard) it. The
// runtime adapter drives skills.drafts.* over RPC.
export interface SkillDraftsGateway {
  promote(handle: SkillDraftHandle): Promise<void>;
  reject(handle: SkillDraftHandle): Promise<void>;
}

const port = createSingletonPort<SkillDraftsGateway>("Skill drafts gateway is not configured");

export const configureSkillDraftsGateway = port.configure;
export const skillDraftsGateway = port.get;
