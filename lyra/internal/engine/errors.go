package engine

import "errors"

// errChatClientMissing surfaces when an action body asks for a chat
// request but the engine was constructed without one. Should never
// happen given [New] rejects nil ChatClient — kept as a defense in
// depth.
var errChatClientMissing = errors.New("engine: chat client missing inside action context")
