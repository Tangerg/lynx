package core

import "github.com/Tangerg/lynx/core/model/chat"

// ChatClient is the LLM client handle the runtime uses for action bodies.
//
// It is intentionally concrete: the domain uses one stable chat protocol type today,
// and narrowing the abstraction here reduces accidental complexity while preserving
// behavior. A nil value means no chat transport is configured for the process.
type ChatClient = *chat.Client
