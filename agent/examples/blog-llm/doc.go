// Command blog-llm extends the basic blog example to use the
// [chat.Client] binding on ProcessContext: an action body grabs the
// platform's shared LLM client via [core.ProcessContext.ChatWithActionTools],
// which returns a request pre-loaded with the action's resolved
// tools and the LLM-driven tool-call middleware. The LLM (a stub
// here, so the example runs offline) calls the tool, gets a result,
// and decides what to write next — all without the action body
// itself implementing a tool loop.
//
// Run from repo root:
//
//	go run ./agent/examples/blog-llm
//
// Wire a real model by replacing newStubModel with a chat.Model
// implementation from one of the lynx/models/* providers.
package main
