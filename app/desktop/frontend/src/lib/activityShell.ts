/**
 * How much of the plane one activity in a transcript claims.
 *
 * Below both the domain and the design system, for the same reason `Tone` is: a view
 * model saying "this call only read a file" is stating a fact, and deciding that such
 * a row therefore wears no card is the disclosure's job. Putting the vocabulary in
 * `ui/agent` made the presentation ring import a component ring to name a fact, which
 * `check-published-boundaries` is right to refuse.
 *
 *   line     A glance — a read, a search, a lookup. There may be a dozen in one turn,
 *            and a dozen cards is the grey stack this vocabulary exists to break up.
 *   card     Something was produced and is worth stopping at.
 *   flagged  Something failed, was refused, or is waiting on a person.
 */
export type ActivityShell = "line" | "card" | "flagged";
