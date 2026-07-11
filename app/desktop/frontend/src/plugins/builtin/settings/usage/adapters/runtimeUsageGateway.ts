import { getContainer } from "@/main/container";
import { configureUsageGateway, type UsageGateway } from "../application/ports/usageGateway";

const gateway: UsageGateway = {
  loadSummary(sinceDays) {
    return getContainer()
      .client()
      .usage.summary(sinceDays > 0 ? { sinceDays } : {});
  },
};

export function installUsageGateway(): void {
  configureUsageGateway(gateway);
}
