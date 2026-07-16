// Command toolloop demonstrates the leaf model/tool protocol directly. Normal
// Agent actions should prefer ProcessContext.Prompt or Interact so the
// Framework owns process binding, accounting, events, suspension, and durable
// continuation around this lower-level Runner.
package main
