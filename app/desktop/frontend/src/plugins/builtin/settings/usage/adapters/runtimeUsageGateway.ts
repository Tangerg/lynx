import { getContainer } from "@/main/container";
import { configureUsageGateway, type UsageGateway } from "../application/ports/usageGateway";

const gateway: UsageGateway = {
  loadSummary(sinceDays, signal) {
    return getContainer()
      .client()
      .usage.summary(sinceDays > 0 ? { sinceDays } : {}, signal);
  },
};

export function installUsageGateway(): () => void {
  return configureUsageGateway(gateway);
}
