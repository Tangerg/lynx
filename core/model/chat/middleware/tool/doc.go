// Package tool turns a chat [chat.Model] into a self-driving tool-calling
// loop. When the model emits tool calls, the loop executes the registered
// tools and re-prompts the model with the results, repeating until the model
// returns a regular reply (or every tool is return-direct, or the iteration
// cap trips). Wire it via [NewMiddleware], which returns the call + stream
// pair.
//
// It sits BELOW the chat protocol: it imports [chat] for the Tool interface,
// Request/Response, and the tool message types, and chat never imports it.
// An application that wants a different control flow can drive [chat.Tool]
// calls itself instead of using this package.
//
// # Error contract
//
// This package decides what a tool's error means — [chat.Tool.Call] only
// produces it. A tool error is RECOVERABLE by default: the loop wraps its
// Error() string in a tool result and feeds it back so the model can adjust
// (retry, fix an argument, tell the user), and the run continues. An
// unregistered tool is likewise answered with an error result rather than
// aborting. The only errors that STOP the loop are control-flow signals, and
// each propagates UNCHANGED so an outer layer can act on it:
//
//   - context cancellation / deadline — the run is being torn down;
//   - a [chat.ToolHalt] whose Abort() is true — aborts the run (a failure the
//     model cannot act on);
//   - a [chat.ToolHalt] whose Abort() is false — parks the run for human input
//     (HITL). The loop hands back a [chat.FinishReasonInterrupt] response — the
//     resumable tail: the round's assistant tool-call message plus any partial
//     results — then propagates the cause. The caller parks and, on resume,
//     feeds that tail back; the loop continues AT the still-pending call (the
//     model is NOT re-invoked for that round, and already-run calls are not
//     re-executed). This is the conversation-as-checkpoint resume — the message
//     shape alone drives it (see resume.go).
//
// None of this is configurable; recovery is the framework default. A tool
// author chooses where an operational failure surfaces — return an ordinary
// error and let the loop fold it in, or fold it into the result string for
// control over the wording. Both reach the model.
package tool
