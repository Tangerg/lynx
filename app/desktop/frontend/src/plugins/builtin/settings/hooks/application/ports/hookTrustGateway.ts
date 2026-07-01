export interface HookTrustGateway {
  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void>;
}

let port: HookTrustGateway | null = null;

export function configureHookTrustGateway(next: HookTrustGateway): void {
  port = next;
}

export function hookTrustGateway(): HookTrustGateway {
  if (!port) throw new Error("Hook trust gateway is not configured");
  return port;
}
