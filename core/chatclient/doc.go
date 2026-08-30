// Package chatclient provides direct, optional conveniences around the minimal
// chat protocols and model capabilities defined by Core. Client.Output binds
// a provider-neutral OutputFormat to the decoder shared by synchronous and
// streaming generations. Client also discovers provider-owned input-token
// counting only when the post-middleware model can measure the same prepared
// request that Call will execute.
//
// [NewToolMiddleware] covers the deliberately small direct-use path: it
// advertises a frozen executable Tool set, validates complete calls, executes
// them serially, and continues the model conversation. Agent control flow,
// retries, concurrency, approval, and durable execution remain outside Core.
package chatclient
