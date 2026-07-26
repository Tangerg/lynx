export type ComposerAction = "send" | "steer" | "stop";

export interface ComposerActionLayout {
  /** The filled circle — the one target the eye goes to. */
  primary: ComposerAction;
  /** A quiet second control, for when the primary is not the way out of a run. */
  secondary: "stop" | null;
}

/**
 * Which controls the composer offers.
 *
 * The rule this exists to hold: **while a run is in flight, stop is always
 * reachable**. Steer and stop shared the single circle, so typing anything during
 * a run replaced the stop button — and the only way to stop was to delete what you
 * had written.
 */
export function composerActionLayout({
  running,
  hasInput,
}: {
  running: boolean;
  hasInput: boolean;
}): ComposerActionLayout {
  if (!running) return { primary: "send", secondary: null };
  if (hasInput) return { primary: "steer", secondary: "stop" };
  return { primary: "stop", secondary: null };
}
