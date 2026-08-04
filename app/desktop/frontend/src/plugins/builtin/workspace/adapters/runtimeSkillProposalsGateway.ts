import { getContainer } from "@/main/container";
import { configureSkillDraftsGateway } from "../application/ports/skillDraftsGateway";
import type { SkillDraftsGateway } from "../application/ports/skillDraftsGateway";

const gateway: SkillDraftsGateway = {
  async promote(handle) {
    await getContainer().client().skills.promoteDraft(handle);
  },
  async reject(handle) {
    await getContainer().client().skills.rejectDraft(handle);
  },
};

export function installSkillDraftsGateway(): () => void {
  return configureSkillDraftsGateway(gateway);
}
