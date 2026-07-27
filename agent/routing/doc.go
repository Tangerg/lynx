// Package routing translates a natural-language prompt into a concrete
// (agent, goal) decision.
//
// Two collaborating types:
//
//   - [Ranker] is the SPI: "given this input, score each candidate goal in
//     [0, 1]". Implement it with a model, with keywords, or with anything
//     else — the prompt and the scoring rubric belong to whoever owns the
//     product, so this package ships no implementation.
//   - [Router] enumerates the engine's active deployments × their goals,
//     asks the Ranker, and applies a confidence cutoff.
//
// A [Choice] names an exact immutable deployment identity. Running it is the
// caller's step — deciding what to run and driving it are different jobs, and
// only the caller knows whether this selection should become a fresh process,
// a child, or nothing at all.
package routing
