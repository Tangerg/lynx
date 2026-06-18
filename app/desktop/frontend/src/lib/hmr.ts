// HMR dispose helper.
//
// Vite's HMR replaces a module's exports in place but doesn't touch
// side effects the module already ran — module-level `subscribe(...)`
// calls, `window.addEventListener(...)`, etc., keep firing through the
// stale closures. Without an explicit dispose hook each reload stacks
// another listener on top of the previous ones; after a hacking
// session the same handler fires N+1 times on every store mutation,
// snowballing into visible jank.
//
// Wrap each cleanup the module wants to release on hot replace:
//
//   const unsub = someStore.subscribe(...);
//   disposeOnHmr(unsub);
//
//   // or multiple:
//   disposeOnHmr(unsub1, unsub2, () => removeEventListener("x", h));
//
// In production builds `import.meta.hot` is undefined, the body is
// dead-code-eliminated, and the helper is a no-op call.

export function disposeOnHmr(...cleanups: Array<() => void>): void {
  if (!import.meta.hot) return;
  import.meta.hot.dispose(() => {
    for (const c of cleanups) c();
  });
}
