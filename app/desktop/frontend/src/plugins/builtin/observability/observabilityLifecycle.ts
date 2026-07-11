export type ObservabilityTeardown = () => void | Promise<void>;

export function startObservability(
  initialize: () => Promise<ObservabilityTeardown>,
  reportFailure: (error: unknown) => void,
): ObservabilityTeardown {
  let disposed = false;
  let teardown: ObservabilityTeardown | null = null;

  void initialize()
    .then((fn) => {
      if (disposed) {
        void fn();
        return;
      }
      teardown = fn;
    })
    .catch(reportFailure);

  return () => {
    disposed = true;
    const fn = teardown;
    teardown = null;
    if (fn) void fn();
  };
}
