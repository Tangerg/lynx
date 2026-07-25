/**
 * Semantic tone — what a piece of state *means*, not what colour it is.
 *
 * Below both the domain and the design system on purpose: a view model saying "this
 * run errored" is stating a fact, and picking the fill and ink that fact wears is
 * the Badge's job. Application layers used to emit Tailwind class strings for
 * this, which put presentation a floor too low and pinned it in their tests.
 */
export type Tone = "neutral" | "accent" | "success" | "warning" | "negative" | "info";
