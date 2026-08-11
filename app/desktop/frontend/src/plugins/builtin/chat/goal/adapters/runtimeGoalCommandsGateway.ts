import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import type { GoalCommandsGateway } from "../application/ports/goalCommandsGateway";
import { configureGoalCommandsGateway } from "../application/ports/goalCommandsGateway";

const gateway: GoalCommandsGateway = {
  async start(input) {
    await getContainer()
      .client()
      .goals.start({ ...input, sessionId: asSessionId(input.sessionId) });
  },
  async stop(sessionId) {
    await getContainer().client().goals.stop(asSessionId(sessionId));
  },
  async resume(sessionId) {
    await getContainer().client().goals.resume(asSessionId(sessionId));
  },
};

export function installGoalCommandsGateway(): () => void {
  return configureGoalCommandsGateway(gateway);
}
