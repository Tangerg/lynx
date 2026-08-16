// "Open the goal launcher, with this already typed into it."
//
// A signal and NOT a store, which is the whole design. A stored request has to be
// cleared by whoever consumes it, and the case that matters here is the one where
// nobody does: the launcher is not on screen while a goal is already running, so a
// request parked in state would sit there and spring the dialog open later, at a
// moment the user has forgotten asking for. Delivering to whoever is listening
// right now has no such state to go stale.
//
// The return value is what lets the caller stay honest. Only a mounted launcher
// subscribes — mounting is already conditioned on the runtime having goals at all
// and on the standing goal being replaceable — so "nothing took it" is the same
// question as "can a goal be set here at present", answered without a second copy
// of the rule that decides it.

type GoalLauncherListener = (objective: string) => void;

const listeners = new Set<GoalLauncherListener>();

/** @returns whether a launcher was listening. False means nothing happened. */
export function requestGoalLauncher(objective: string): boolean {
  // Copied before iterating: a listener that unsubscribes while opening would
  // otherwise mutate the set mid-loop.
  for (const listener of [...listeners]) listener(objective);
  return listeners.size > 0;
}

export function onGoalLauncherRequest(listener: GoalLauncherListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
