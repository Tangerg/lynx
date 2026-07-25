// The class that marks a rendered message body. It is a genuine cross-cutting
// contract, not a styling detail: globals.css hangs the selection scope, the
// markdown overrides and the image theming off it, and chat search walks the
// same subtree to find text ranges. Named here, in the context that renders it,
// so a consumer imports the fact instead of re-typing the string — a duplicated
// selector is a dependency that no import guard can see.
export const MESSAGE_CONTENT_CLASS = "msg-content";
export const MESSAGE_CONTENT_SELECTOR = `.${MESSAGE_CONTENT_CLASS}`;
