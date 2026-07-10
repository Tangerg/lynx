export type BootstrapTeardown = () => void | Promise<void>;

export interface BootstrapLifecyclePorts {
  installPorts: () => void;
  initObservability: () => Promise<BootstrapTeardown>;
  discoverRuntime: () => Promise<void>;
  reportObservabilityFailure: (error: unknown) => void;
  reportDiscoveryFailure: (error: unknown) => void;
}

export function startBootstrapLifecycle(ports: BootstrapLifecyclePorts): BootstrapTeardown {
  ports.installPorts();

  let disposed = false;
  let teardown: BootstrapTeardown | null = null;

  void ports
    .initObservability()
    .then((fn) => {
      if (disposed) {
        void fn();
        return;
      }
      teardown = fn;
    })
    .catch(ports.reportObservabilityFailure);

  void ports.discoverRuntime().catch(ports.reportDiscoveryFailure);

  return () => {
    disposed = true;
    const fn = teardown;
    teardown = null;
    if (fn) void fn();
  };
}
