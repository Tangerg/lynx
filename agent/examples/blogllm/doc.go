// Command blogllm extends the basic blog example to use the
// [chatclient.Client] binding on ProcessContext: an action body grabs the
// engine's shared LLM client via [core.ProcessContext.Prompt],
// which returns a request pre-loaded with the action's resolved
// tools and the runtime-owned Event Runner. The LLM (a stub
// here, so the example runs offline) calls the tool, gets a result,
// and decides what to write next — all without the action body
// itself implementing a tool loop.
//
// Run from repo root:
//
//	go run ./agent/examples/blogllm
//
// Wire a real model by replacing newStubModel with a chat.Model
// implementation from one of the lynx/models/* providers.
package main
