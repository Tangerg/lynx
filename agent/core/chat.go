package core

import "github.com/Tangerg/lynx/core/model/chat"

// ChatClient is the minimal LLM request factory the agent core needs.
//
// The returned request is the shared chat protocol builder, not a concrete
// transport. Keeping the port this narrow lets runtimes choose the client per
// process while preserving the existing chat tool/options/middleware surface
// action bodies already compose with.
type ChatClient interface {
	Chat() *chat.ClientRequest
}
