// Package runmaintenance implements the post-Run maintenance pipeline: history
// compaction, long-term memory consolidation, and governed Skill learning.
//
// These workers operate OUTSIDE the normal conversation flow — they call the
// utility model directly, bypassing chat history, tools, and guardrails so their
// own prompts never enter the interactive conversation. The workers share only
// this package's transcript rendering; the generic middleware-free call belongs
// to adapter/utilitymodel.
//
// Pipeline owns ordering and failure aggregation for a clean Run boundary. The
// execution adapter supplies finished-Run facts and observes the result.
package runmaintenance
