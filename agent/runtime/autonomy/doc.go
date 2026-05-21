// Package autonomy translates a natural-language user prompt into a
// concrete (agent, goal) decision and runs it.
//
// Two collaborating types:
//
//   - [Ranker] is the SPI: "given this user input, score each
//     candidate goal in [0, 1]". Plug an [LLMRanker] for chat-driven
//     ranking, a regex/keyword ranker for cheap routing, or a hybrid.
//   - [Autonomy] is the orchestrator: it enumerates the platform's
//     deployed agents × their goals, asks the Ranker, applies a
//     confidence cutoff, and (via [Autonomy.Run]) launches the
//     winning agent with a per-process [core.GoalApprover] that
//     locks the planner onto just the chosen goal.
//
// Mirrors embabel's `Autonomy` + `Ranker` SPI without the Spring DI
// scaffolding. lynx ships [LLMRanker] as the canonical LLM-backed
// ranker; users with simpler routing rules can implement [Ranker]
// directly.
package autonomy
