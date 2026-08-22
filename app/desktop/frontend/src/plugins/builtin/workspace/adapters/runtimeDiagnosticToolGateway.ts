import { getContainer } from "@/main/container";
import { DiagnosticToolOwner } from "../application/diagnosticTool";
import type { DiagnosticToolGateway } from "../application/ports/diagnosticToolGateway";

export function installDiagnosticToolGateway() {
  const client = getContainer().client();
  const gateway: DiagnosticToolGateway = {
    invoke(input) {
      return client.tools.invoke({
        name: input.name,
        arguments: input.arguments,
        ...(input.cwd ? { workspace: { path: input.cwd } } : {}),
      });
    },
  };
  const owner = DiagnosticToolOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
