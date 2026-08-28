// Package chatclient provides direct, optional conveniences around the minimal
// chat protocols and model capabilities defined by Core. Client.Output binds
// a provider-neutral OutputFormat to the decoder shared by synchronous and
// streaming generations. Client also discovers provider-owned input-token
// counting only when the post-middleware model can measure the same prepared
// request that Call will execute.
package chatclient
