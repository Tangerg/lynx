import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { configureGoalGateway } from "../application/ports/goalGateway";
import type { GoalGateway } from "../application/ports/goalGateway";

const gateway: GoalGateway = {
  async start(input) {
    await getContainer()
      .client()
      .goals.start({
        sessionId: asSessionId(input.sessionId),
        objective: input.objective,
        budget: input.budget,
      });
  },
  async stop(sessionId) {
    await getContainer().client().goals.stop(asSessionId(sessionId));
  },
  async resume(sessionId) {
    await getContainer().client().goals.resume(asSessionId(sessionId));
  },
};

export function installGoalGateway(): () => void {
  return configureGoalGateway(gateway);
}
