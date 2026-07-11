import { createSingletonPort } from "@/lib/ports/singletonPort";
export interface HookTrustGateway {
  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void>;
}

const port = createSingletonPort<HookTrustGateway>("Hook trust gateway is not configured");

export const configureHookTrustGateway = port.configure;
export const hookTrustGateway = port.get;
