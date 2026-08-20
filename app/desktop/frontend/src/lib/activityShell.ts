/**
 * How much of the plane one activity in a transcript claims.
 *
 * Below both the domain and the design system, for the same reason `Tone` is: a view
 * model saying "this call only read a file" is stating a fact, and deciding that such
 * a row therefore wears no card is the disclosure's job. Putting the vocabulary in
 * `ui/agent` made the presentation ring import a component ring to name a fact, which
 * `check-published-boundaries` is right to refuse.
 *
 *   line     Work-narrative activity. Tool invocations stay here in every lifecycle
 *            state; their disclosed material owns any terminal/diff surface.
 *   card     A composite product with a narrative of its own, such as a delegated Run.
 *   flagged  A composite card whose own boundary needs attention.
 */
export type ActivityShell = "line" | "card" | "flagged";
