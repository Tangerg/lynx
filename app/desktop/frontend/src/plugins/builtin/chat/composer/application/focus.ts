// Focusing the composer is a capability of the composer, not a DOM lookup any
// caller may perform. Callers cannot depend on its internal selector or mount shape.
//
// A module-level handle rather than a threaded ref: there is exactly one composer
// per window, and the callers sit on the far side of two context boundaries. A
// ref would have to travel through both, which is how the shortcut ended up
// reaching for a class name instead.

let target: HTMLTextAreaElement | null = null;

/** Called by the composer's own input controller for as long as it is mounted. */
export function setComposerFocusTarget(element: HTMLTextAreaElement | null): void {
  target = element;
}

/**
 * Focus the composer's input. `selectionEnd` collapses the caret there — used
 * when text is loaded back in for editing, so the user continues at the end of
 * what they wrote rather than the start.
 */
export function focusComposer(selectionEnd?: number): void {
  const element = target;
  if (!element) return;
  element.focus();
  if (selectionEnd !== undefined) element.setSelectionRange(selectionEnd, selectionEnd);
}
